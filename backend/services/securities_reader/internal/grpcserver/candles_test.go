package grpcserver

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/securities_reader/internal/storage"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

func (f *fakeStore) BoardsOf(_ context.Context, secid string) ([]string, error) {
	return f.boards[secid], nil
}

func (f *fakeStore) LatestCandleStart(_ context.Context, _, _, interval string) (time.Time, bool, error) {
	c := f.candles[interval]
	if len(c) == 0 {
		return time.Time{}, false, nil
	}
	return c[len(c)-1].Start, true, nil
}

func (f *fakeStore) Candles(_ context.Context, _, _, interval string, from time.Time) ([]storage.Candle, error) {
	f.gotInterval, f.gotFrom = interval, from
	var out []storage.Candle
	for _, c := range f.candles[interval] {
		if !c.Start.Before(from) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeStore) WeeklyCandles(context.Context, string, string) ([]storage.Candle, error) {
	return f.weekly, nil
}

var msk = time.FixedZone("MSK", 3*3600)

func candlesServer(store *fakeStore, now time.Time) *Server {
	s := newTestServer(store)
	s.Loc = msk
	s.Now = func() time.Time { return now }
	return s
}

func at(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, msk) }

func TestGetCandles_DayUsesLastTradingDayHours(t *testing.T) {
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		candles: map[string][]storage.Candle{
			storage.IntervalHour: {{Start: at(25, 18), Close: 1}, {Start: at(26, 10), Close: 2}, {Start: at(26, 11), Close: 3}},
		},
	}
	
	s := candlesServer(store, at(27, 12))

	resp, err := s.GetCandles(context.Background(), &securitiesreaderpb.GetCandlesRequest{Secid: " sber ", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_DAY})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Board != "TQBR" || resp.Secid != "SBER" || resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_HOUR {
		t.Errorf("resp = %+v", resp)
	}
	if len(resp.Candles) != 2 || resp.Candles[0].Close != 2 {
		t.Errorf("candles = %+v, want the 2 candles of 26 Sep", resp.Candles)
	}
}

func TestGetCandles_DayWithoutDataIsEmpty(t *testing.T) {
	s := candlesServer(&fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}}, at(24, 12))
	resp, err := s.GetCandles(context.Background(), &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_DAY})
	if err != nil || len(resp.Candles) != 0 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}

func TestGetCandles_WeekUsesDaily(t *testing.T) {
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		candles: map[string][]storage.Candle{
			storage.IntervalHour: {{Start: at(24, 10)}},
			storage.IntervalDay:  {{Start: at(16, 0)}, {Start: at(20, 0)}, {Start: at(23, 0)}},
		},
	}
	resp, err := candlesServer(store, at(24, 12)).GetCandles(context.Background(), &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_WEEK})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_DAY || len(resp.Candles) != 2 {
		t.Errorf("interval=%v candles=%d", resp.Interval, len(resp.Candles))
	}
	if !store.gotFrom.Equal(at(17, 0)) {
		t.Errorf("from = %v, want start of day a week ago", store.gotFrom)
	}
}

func TestGetCandles_MonthYearAll(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, weekly: []storage.Candle{{Start: at(21, 0)}}}
	s := candlesServer(store, time.Date(2026, 9, 24, 12, 30, 0, 0, msk))

	cases := []struct {
		r        securitiesreaderpb.CandleRange
		from     time.Time
		interval securitiesreaderpb.CandleInterval
	}{
		{securitiesreaderpb.CandleRange_CANDLE_RANGE_MONTH, time.Date(2026, 8, 24, 0, 0, 0, 0, msk), securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_DAY},
		{securitiesreaderpb.CandleRange_CANDLE_RANGE_YEAR, time.Date(2025, 9, 24, 0, 0, 0, 0, msk), securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_DAY},
	}
	for _, c := range cases {
		resp, err := s.GetCandles(context.Background(), &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: c.r})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Interval != c.interval || store.gotInterval != storage.IntervalDay || !store.gotFrom.Equal(c.from) {
			t.Errorf("%v: interval=%v asked %s from %v", c.r, resp.Interval, store.gotInterval, store.gotFrom)
		}
	}

	resp, err := s.GetCandles(context.Background(), &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_ALL})
	if err != nil || resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_WEEK || len(resp.Candles) != 1 {
		t.Errorf("ALL: resp=%v err=%v", resp, err)
	}
}

func TestGetCandles_Errors(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}, "DUAL": {"TQBR", "TQTF"}}}
	s := candlesServer(store, at(24, 12))
	day := securitiesreaderpb.CandleRange_CANDLE_RANGE_DAY

	cases := []struct {
		name string
		req  *securitiesreaderpb.GetCandlesRequest
		code codes.Code
	}{
		{"no secid", &securitiesreaderpb.GetCandlesRequest{Range: day}, codes.InvalidArgument},
		{"no range", &securitiesreaderpb.GetCandlesRequest{Secid: "SBER"}, codes.InvalidArgument},
		{"bad range", &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: 42}, codes.InvalidArgument},
		{"unknown security", &securitiesreaderpb.GetCandlesRequest{Secid: "NOPE", Range: day}, codes.NotFound},
		{"wrong board", &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Board: "TQTF", Range: day}, codes.NotFound},
		{"ambiguous board", &securitiesreaderpb.GetCandlesRequest{Secid: "DUAL", Range: day}, codes.InvalidArgument},
	}
	for _, c := range cases {
		_, err := s.GetCandles(context.Background(), c.req)
		if status.Code(err) != c.code {
			t.Errorf("%s: code = %v, want %v", c.name, status.Code(err), c.code)
		}
	}

	resp, err := s.GetCandles(context.Background(), &securitiesreaderpb.GetCandlesRequest{Secid: "DUAL", Board: "tqtf", Range: day})
	if err != nil || resp.Board != "TQTF" {
		t.Errorf("explicit board: resp=%v err=%v", resp, err)
	}
}
