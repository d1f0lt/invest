package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/securities_reader/internal/dohod"
	"invest/backend/services/securities_reader/internal/moex"
	"invest/backend/services/securities_reader/internal/storage"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

func (f *fakeStore) SecurityRef(_ context.Context, secid, board string) (storage.SecurityRef, bool, error) {
	r, ok := f.refs[secid+"|"+board]
	return r, ok, nil
}

type fakeMoex struct {
	candles     []moex.Candle
	candlesErr  error
	gotDay      time.Time
	candleCalls int
	gotInterval int
	gotFrom     time.Time

	desc    []moex.DescriptionField
	descErr error
	issuer  string

	descCall int
}

func (m *fakeMoex) IntradayCandles(_ context.Context, _, _ string, day time.Time, _ *time.Location) ([]moex.Candle, error) {
	m.candleCalls++
	m.gotDay = day
	return m.candles, m.candlesErr
}

func (m *fakeMoex) Candles(_ context.Context, _, _ string, interval int, from, _ time.Time, _ *time.Location) ([]moex.Candle, error) {
	m.candleCalls++
	m.gotInterval, m.gotFrom = interval, from
	return m.candles, m.candlesErr
}

func (m *fakeMoex) Description(context.Context, string) ([]moex.DescriptionField, error) {
	m.descCall++
	return m.desc, m.descErr
}

func (m *fakeMoex) EmitterTitle(context.Context, string) (string, error) {
	return m.issuer, nil
}

func dayRequest() *securitiesreaderpb.GetCandlesRequest {
	return &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_DAY}
}

func TestGetCandles_DayFromMoexForLastTradingDay(t *testing.T) {
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		candles: map[string][]storage.Candle{
			storage.IntervalDay:  {{Start: at(25, 0)}},
			storage.IntervalHour: {{Start: at(25, 13), Close: 9}},
		},
	}
	m := &fakeMoex{candles: []moex.Candle{
		{Begin: at(25, 10), Open: 1, High: 2, Low: 1, Close: 2},
		{Begin: at(25, 10).Add(10 * time.Minute), Open: 2, High: 3, Low: 2, Close: 3},
	}}
	s := candlesServer(store, at(26, 12))
	s.Moex = m

	resp, err := s.GetCandles(context.Background(), dayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_TEN_MINUTES || len(resp.Candles) != 2 || resp.Candles[1].Close != 3 {
		t.Errorf("resp = %+v", resp)
	}
	if !m.gotDay.Equal(at(25, 0)) {
		t.Errorf("day = %v, want last daily candle 25 Sep", m.gotDay)
	}

	if _, err := s.GetCandles(context.Background(), dayRequest()); err != nil {
		t.Fatal(err)
	}
	if m.candleCalls != 1 {
		t.Errorf("moex called %d times, want 1 (cached)", m.candleCalls)
	}
}

func TestGetCandles_DayFallsBackToHourlyWhenMoexFails(t *testing.T) {
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		candles: map[string][]storage.Candle{
			storage.IntervalHour: {{Start: at(26, 10), Close: 2}},
		},
	}
	s := candlesServer(store, at(26, 12))
	s.Moex = &fakeMoex{candlesErr: errors.New("403")}

	resp, err := s.GetCandles(context.Background(), dayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_HOUR || len(resp.Candles) != 1 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestGetSecurityInfo_MergesMoexAndDatabase(t *testing.T) {
	name, isin := "Сбербанк России ПАО ао", "RU0009029540"
	lot := int64(10)
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		refs:   map[string]storage.SecurityRef{"SBER|TQBR": {SecName: &name, ISIN: &isin, LotSize: &lot}},
	}
	m := &fakeMoex{
		issuer: "ПАО Сбербанк",
		desc: []moex.DescriptionField{
			{Name: "TYPENAME", Value: "Акция обыкновенная"},
			{Name: "FACEVALUE", Value: "3"},
			{Name: "FACEUNIT", Value: "SUR"},
			{Name: "ISSUESIZE", Value: "21586948000"},
			{Name: "LISTLEVEL", Value: "1"},
			{Name: "ISQUALIFIEDINVESTORS", Value: "0"},
			{Name: "MATDATE", Value: ""},
		},
	}
	s := candlesServer(store, at(26, 12))
	s.Moex = m

	resp, err := s.GetSecurityInfo(context.Background(), &securitiesreaderpb.GetSecurityInfoRequest{Secid: "sber"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range resp.Fields {
		names = append(names, f.Name)
	}
	want := []string{"ISSUER", "NAME", "TYPE", "ISIN", "LOTSIZE", "ISSUESIZE", "LISTLEVEL"}
	if len(names) != len(want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("fields = %v, want %v", names, want)
		}
	}
	if is := resp.Fields[5]; is.GetUnit() != "шт." || is.Type != "number" {
		t.Errorf("issue size = %+v", is)
	}

	if _, err := s.GetSecurityInfo(context.Background(), &securitiesreaderpb.GetSecurityInfoRequest{Secid: "SBER"}); err != nil {
		t.Fatal(err)
	}
	if m.descCall != 1 {
		t.Errorf("moex description called %d times, want 1 (cached)", m.descCall)
	}
}

func TestGetSecurityInfo_MoexDownStillReturnsDatabaseFieldsUncached(t *testing.T) {
	name := "Сбербанк России ПАО ао"
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		refs:   map[string]storage.SecurityRef{"SBER|TQBR": {SecName: &name}},
	}
	m := &fakeMoex{descErr: errors.New("timeout")}
	s := candlesServer(store, at(26, 12))
	s.Moex = m

	for i := 0; i < 2; i++ {
		resp, err := s.GetSecurityInfo(context.Background(), &securitiesreaderpb.GetSecurityInfoRequest{Secid: "SBER"})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Fields) != 1 || resp.Fields[0].Name != "NAME" {
			t.Errorf("fields = %+v", resp.Fields)
		}
	}
	if m.descCall != 2 {
		t.Errorf("moex description called %d times, want 2 (errors are not cached)", m.descCall)
	}
}

func TestGetCandles_FiveYearsFromMoexWeekly(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, weekly: []storage.Candle{{Start: at(21, 0)}}}
	m := &fakeMoex{candles: []moex.Candle{{Begin: time.Date(2021, 9, 27, 0, 0, 0, 0, msk), Close: 1}, {Begin: at(21, 0), Close: 2}}}
	s := candlesServer(store, at(26, 12))
	s.Moex = m

	req := &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_FIVE_YEARS}
	resp, err := s.GetCandles(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_WEEK || len(resp.Candles) != 2 {
		t.Errorf("resp = %+v", resp)
	}
	if m.gotInterval != 7 || !m.gotFrom.Equal(time.Date(2021, 9, 26, 0, 0, 0, 0, msk)) {
		t.Errorf("interval=%d from=%v", m.gotInterval, m.gotFrom)
	}
	if _, err := s.GetCandles(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if m.candleCalls != 1 {
		t.Errorf("moex called %d times, want 1 (cached)", m.candleCalls)
	}

	s2 := candlesServer(store, at(26, 12))
	s2.Moex = &fakeMoex{candlesErr: errors.New("403")}
	resp, err = s2.GetCandles(context.Background(), req)
	if err != nil || len(resp.Candles) != 1 {
		t.Errorf("fallback resp=%v err=%v", resp, err)
	}
}

func TestShortIssuerName(t *testing.T) {
	for in, want := range map[string]string{
		`Публичное акционерное общество "Сбербанк России"`:   "ПАО «Сбербанк России»",
		`  публичное  акционерное общество "Газпром"`:        "ПАО «Газпром»",
		`Акционерное общество "Тинькофф Банк"`:               "АО «Тинькофф Банк»",
		`Министерство финансов Российской Федерации`:         "Министерство финансов Российской Федерации",
		`Общество с ограниченной ответственностью "Ромашка"`: "ООО «Ромашка»",
		`ПАО "a "b"`: `ПАО "a "b"`,
	} {
		if got := shortIssuerName(in); got != want {
			t.Errorf("shortIssuerName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildInfoFields_FaceValueOnlyForBonds(t *testing.T) {
	values := map[string]string{"FACEVALUE": "1000", "FACEUNIT": "SUR"}
	if got := buildInfoFields(values, false); len(got) != 0 {
		t.Errorf("share fields = %+v, want no face value", got)
	}
	got := buildInfoFields(values, true)
	if len(got) != 1 || got[0].Name != "FACEVALUE" || got[0].GetUnit() != "SUR" {
		t.Errorf("bond fields = %+v", got)
	}
}

type fakeDividends struct {
	divs  []dohod.Dividend
	err   error
	calls int
}

func (f *fakeDividends) Dividends(context.Context, string) ([]dohod.Dividend, error) {
	f.calls++
	return f.divs, f.err
}

func TestGetDividends_DedupedCachedWithForecast(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}}
	src := &fakeDividends{divs: []dohod.Dividend{
		{RegistryCloseDate: "2027-07-20", Value: 44.53, Currency: "RUB", Forecast: true},
		{RegistryCloseDate: "2026-07-20", Value: 37.64, Currency: "RUB", DeclaredDate: "2026-04-21"},
		{RegistryCloseDate: "2026-07-20", Value: 37.64, Currency: "RUB"},
		{RegistryCloseDate: "2025-07-18", Value: 34.84, Currency: "RUB"},
	}}
	s := candlesServer(store, at(26, 12))
	s.DividendSource = src

	resp, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: " sber "})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Secid != "SBER" || len(resp.Dividends) != 3 || !resp.Dividends[0].Forecast ||
		resp.Dividends[1].DeclaredDate != "2026-04-21" || resp.Dividends[1].Forecast {
		t.Errorf("resp = %+v", resp)
	}
	if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "SBER"}); err != nil {
		t.Fatal(err)
	}
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1 (cached)", src.calls)
	}
}

func TestGetDividends_BondsSkipSource(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SU26238RMFS4": {"TQOB"}}}
	src := &fakeDividends{}
	s := candlesServer(store, at(26, 12))
	s.DividendSource = src
	resp, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "SU26238RMFS4"})
	if err != nil || len(resp.Dividends) != 0 || src.calls != 0 {
		t.Errorf("resp=%v err=%v calls=%d", resp, err, src.calls)
	}
}

func TestGetDividends_Errors(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}}
	src := &fakeDividends{err: errors.New("timeout")}
	s := candlesServer(store, at(26, 12))
	s.DividendSource = src

	if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "NOPE"}); status.Code(err) != codes.NotFound {
		t.Errorf("unknown: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "SBER"}); status.Code(err) != codes.Unavailable {
			t.Errorf("source down: %v", err)
		}
	}
	if src.calls != 2 {
		t.Errorf("errors must not be cached, calls = %d", src.calls)
	}
	if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty: %v", err)
	}
}
