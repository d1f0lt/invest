package updater

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"invest/backend/services/price_updater/internal/moexclient"
	"invest/backend/services/price_updater/internal/storage"
)

var msk = time.FixedZone("MSK", 3*3600)

func fp(v float64) *float64 { return &v }
func sp(v string) *string   { return &v }

func TestBuildRows(t *testing.T) {
	collected := time.Date(2026, 9, 24, 11, 37, 0, 0, time.UTC) 
	quotes := map[string]moexclient.MarketQuote{
		"SBER":   {Last: fp(305), Open: fp(300), High: fp(306), Low: fp(299), TradingStatus: sp("T")},
		"CLOSED": {Last: fp(100), Open: fp(99), TradingStatus: sp("N")},
		"BOND":   {Last: fp(98.5), TradingStatus: sp("T")}, 
		"NOLAST": {TradingStatus: sp("T")},
	}

	prices, hourly, daily := buildRows("TQBR", quotes, collected, msk)

	if len(prices) != 4 {
		t.Errorf("prices = %d, want 4 (every quote keeps a current-price row)", len(prices))
	}
	if len(hourly) != 2 {
		t.Fatalf("hourly = %d, want 2 (SBER, BOND): %+v", len(hourly), hourly)
	}
	wantHour := time.Date(2026, 9, 24, 14, 0, 0, 0, msk)
	for _, c := range hourly {
		if !c.StartAt.Equal(wantHour) || c.Interval != storage.IntervalHour {
			t.Errorf("hourly candle %+v, want start %v", c, wantHour)
		}
		if c.Open != c.Close || c.High != c.Close || c.Low != c.Close {
			t.Errorf("a single observation must be a flat candle: %+v", c)
		}
	}
	if len(daily) != 1 || daily[0].SecID != "SBER" {
		t.Fatalf("daily = %+v, want only SBER", daily)
	}
	d := daily[0]
	if !d.StartAt.Equal(time.Date(2026, 9, 24, 0, 0, 0, 0, msk)) {
		t.Errorf("daily start = %v", d.StartAt)
	}
	if d.Open != 300 || d.High != 306 || d.Low != 299 || d.Close != 305 {
		t.Errorf("daily = %+v", d)
	}
}

func TestBuildRowsHourBoundaryUsesMoscowTime(t *testing.T) {
	collected := time.Date(2026, 9, 23, 21, 5, 0, 0, time.UTC) 
	_, hourly, daily := buildRows("TQBR", map[string]moexclient.MarketQuote{
		"SBER": {Last: fp(1), Open: fp(1), TradingStatus: sp("T")},
	}, collected, msk)
	if !hourly[0].StartAt.Equal(time.Date(2026, 9, 24, 0, 0, 0, 0, msk)) {
		t.Errorf("hour start = %v", hourly[0].StartAt.In(msk))
	}
	if !daily[0].StartAt.Equal(time.Date(2026, 9, 24, 0, 0, 0, 0, msk)) {
		t.Errorf("day start = %v", daily[0].StartAt.In(msk))
	}
}

func TestNextDailyRun(t *testing.T) {
	cases := []struct {
		now, want time.Time
	}{
		{time.Date(2026, 9, 24, 1, 0, 0, 0, msk), time.Date(2026, 9, 24, 3, 0, 0, 0, msk)},
		{time.Date(2026, 9, 24, 3, 0, 0, 0, msk), time.Date(2026, 9, 25, 3, 0, 0, 0, msk)},
		{time.Date(2026, 9, 24, 23, 0, 0, 0, msk), time.Date(2026, 9, 25, 3, 0, 0, 0, msk)},
		{time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 3, 0, 0, 0, msk)},
	}
	for _, c := range cases {
		if got := nextDailyRun(c.now, msk, 3, 0); !got.Equal(c.want) {
			t.Errorf("nextDailyRun(%v) = %v, want %v", c.now, got, c.want)
		}
	}
}

type fakeClient struct {
	history map[string][]moexclient.DailyCandle 
	asked   []string
}

func (f *fakeClient) FetchBoard(context.Context, string) (moexclient.BoardSnapshot, error) {
	return moexclient.BoardSnapshot{}, nil
}

func (f *fakeClient) FetchBoardHistory(_ context.Context, _ string, d time.Time) ([]moexclient.DailyCandle, error) {
	key := d.Format("2006-01-02")
	f.asked = append(f.asked, key)
	return f.history[key], nil
}

type fakeStore struct {
	synced  map[string]time.Time
	candles []storage.Candle
	pruned  bool
}

func (f *fakeStore) UpsertSecurities(context.Context, []moexclient.Security) error { return nil }
func (f *fakeStore) UpsertLatestPrices(context.Context, []storage.PriceRow) (int64, error) {
	return 0, nil
}
func (f *fakeStore) UpsertCandles(_ context.Context, c []storage.Candle, _ storage.CandleMode) (int64, error) {
	f.candles = append(f.candles, c...)
	return int64(len(c)), nil
}
func (f *fakeStore) HistorySyncedThrough(_ context.Context, b string) (time.Time, bool, error) {
	d, ok := f.synced[b]
	return d, ok, nil
}
func (f *fakeStore) SetHistorySyncedThrough(_ context.Context, b string, d time.Time) error {
	f.synced[b] = d
	return nil
}
func (f *fakeStore) PruneHourlyCandles(context.Context) (int64, error) {
	f.pruned = true
	return 0, nil
}

func day(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

func newJob(c *fakeClient, s *fakeStore, now time.Time, backfill int) *HistoryJob {
	j := NewHistoryJob(c, s, []string{"TQBR"}, msk, HistoryConfig{RunAtHour: 3, BackfillDays: backfill},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	j.now = func() time.Time { return now }
	return j
}

func TestHistoryJobCatchesUpFromLastSyncedDate(t *testing.T) {
	c := &fakeClient{history: map[string][]moexclient.DailyCandle{
		"2026-09-22": {{SecID: "SBER", BoardID: "TQBR", TradeDate: day("2026-09-22"), Open: 1, High: 2, Low: 1, Close: 2}},
		"2026-09-23": {{SecID: "SBER", BoardID: "TQBR", TradeDate: day("2026-09-23"), Open: 2, High: 3, Low: 2, Close: 3}},
	}}
	s := &fakeStore{synced: map[string]time.Time{"TQBR": day("2026-09-20")}}
	now := time.Date(2026, 9, 24, 3, 0, 0, 0, msk)

	newJob(c, s, now, 365).RunOnce(context.Background())

	if want := []string{"2026-09-21", "2026-09-22", "2026-09-23"}; len(c.asked) != 3 || c.asked[0] != want[0] || c.asked[2] != want[2] {
		t.Errorf("asked dates = %v, want %v", c.asked, want)
	}
	if !s.synced["TQBR"].Equal(day("2026-09-23")) {
		t.Errorf("synced through %v", s.synced["TQBR"])
	}
	if len(s.candles) != 2 {
		t.Fatalf("candles = %d", len(s.candles))
	}
	if got := s.candles[1].StartAt; !got.Equal(time.Date(2026, 9, 23, 0, 0, 0, 0, msk)) {
		t.Errorf("daily candle start = %v, want 00:00 MSK of the trade date", got)
	}
	if !s.pruned {
		t.Error("hourly candles were not pruned")
	}
}

func TestHistoryJobRetriesEmptyYesterday(t *testing.T) {
	c := &fakeClient{history: map[string][]moexclient.DailyCandle{}} 
	s := &fakeStore{synced: map[string]time.Time{"TQBR": day("2026-09-21")}}
	now := time.Date(2026, 9, 24, 3, 0, 0, 0, msk)

	newJob(c, s, now, 365).RunOnce(context.Background())

	if !s.synced["TQBR"].Equal(day("2026-09-22")) {
		t.Errorf("synced through %v, want 2026-09-22 (empty yesterday must stay unsynced)", s.synced["TQBR"])
	}
}

func TestHistoryJobBackfillsNewBoard(t *testing.T) {
	c := &fakeClient{history: map[string][]moexclient.DailyCandle{
		"2026-09-23": {{SecID: "SBER", BoardID: "TQBR", TradeDate: day("2026-09-23"), Open: 1, High: 1, Low: 1, Close: 1}},
	}}
	s := &fakeStore{synced: map[string]time.Time{}}
	newJob(c, s, time.Date(2026, 9, 24, 3, 0, 0, 0, msk), 3).RunOnce(context.Background())

	if len(c.asked) != 3 || c.asked[0] != "2026-09-21" {
		t.Errorf("asked = %v, want 3 days starting 2026-09-21", c.asked)
	}
}
