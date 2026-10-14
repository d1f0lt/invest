package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/securities_reader/internal/storage"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

func (f *fakeStore) SecurityRef(_ context.Context, secid, board string) (storage.SecurityRef, bool, error) {
	r, ok := f.refs[secid+"|"+board]
	return r, ok, nil
}

func (f *fakeStore) SecurityMetadata(context.Context, string) (storage.SecurityMetadata, error) {
	return f.metadata, f.metadataErr
}
func (f *fakeStore) Dividends(context.Context, string) ([]storage.Dividend, bool, error) {
	f.dividendReads++
	return f.dividends, f.dividendsLoaded, f.dividendsErr
}

func dayRequest() *securitiesreaderpb.GetCandlesRequest {
	return &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_DAY}
}

func TestGetCandles_DayReadsDatabaseWithoutMoex(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, candles: map[string][]storage.Candle{
		storage.IntervalTenMinutes: {{Start: at(24, 10), Close: 1}, {Start: at(25, 10), Close: 2}, {Start: at(25, 10).Add(10 * time.Minute), Close: 3}},
	}}
	s := candlesServer(store, at(26, 12))
	resp, err := s.GetCandles(context.Background(), dayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_TEN_MINUTES || len(resp.Candles) != 2 || resp.Candles[1].Close != 3 {
		t.Fatalf("resp = %+v", resp)
	}
	if !store.gotFrom.Equal(at(25, 0)) {
		t.Fatalf("from = %v", store.gotFrom)
	}
}

func TestGetCandles_NewTradingDayUsesFreshHourlyUntilIntradayArrives(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, candles: map[string][]storage.Candle{
		storage.IntervalTenMinutes: {{Start: at(25, 18), Close: 1}},
		storage.IntervalHour:       {{Start: at(26, 10), Close: 2}},
	}}
	s := candlesServer(store, at(26, 12))
	resp, err := s.GetCandles(context.Background(), dayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_HOUR || len(resp.Candles) != 1 || resp.Candles[0].Close != 2 {
		t.Fatalf("resp=%+v", resp)
	}
	store.candles[storage.IntervalTenMinutes] = append(store.candles[storage.IntervalTenMinutes], storage.Candle{Start: at(26, 10), Close: 3})
	resp, err = s.GetCandles(context.Background(), dayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_TEN_MINUTES || len(resp.Candles) != 1 || resp.Candles[0].Close != 3 {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestGetCandles_DayFallsBackToStoredHourly(t *testing.T) {
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		candles: map[string][]storage.Candle{
			storage.IntervalHour: {{Start: at(26, 10), Close: 2}},
		},
	}
	s := candlesServer(store, at(26, 12))

	resp, err := s.GetCandles(context.Background(), dayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_HOUR || len(resp.Candles) != 1 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestGetSecurityInfo_MergesStoredMetadataAndReference(t *testing.T) {
	name, isin := "Сбербанк России ПАО ао", "RU0009029540"
	lot := int64(10)
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		refs:   map[string]storage.SecurityRef{"SBER|TQBR": {SecName: &name, ISIN: &isin, LotSize: &lot}},
	}

	store.metadata = storage.SecurityMetadata{Issuer: "ПАО Сбербанк", Values: map[string]string{
		"TYPENAME": "Акция обыкновенная", "FACEVALUE": "3", "FACEUNIT": "SUR", "ISSUESIZE": "21586948000", "LISTLEVEL": "1", "ISQUALIFIEDINVESTORS": "0",
	}}
	s := candlesServer(store, at(26, 12))

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
}

func TestGetSecurityInfo_BeforeMetadataLoadReturnsReferenceFields(t *testing.T) {
	name := "Сбербанк России ПАО ао"
	store := &fakeStore{
		boards: map[string][]string{"SBER": {"TQBR"}},
		refs:   map[string]storage.SecurityRef{"SBER|TQBR": {SecName: &name}},
	}
	s := candlesServer(store, at(26, 12))

	for i := 0; i < 2; i++ {
		resp, err := s.GetSecurityInfo(context.Background(), &securitiesreaderpb.GetSecurityInfoRequest{Secid: "SBER"})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Fields) != 1 || resp.Fields[0].Name != "NAME" {
			t.Errorf("fields = %+v", resp.Fields)
		}
	}
}

func TestGetCandles_FiveYearsFromStoredWeekly(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, weekly: []storage.Candle{
		{Start: time.Date(2020, 1, 6, 0, 0, 0, 0, msk), Close: 1},
		{Start: time.Date(2021, 9, 27, 0, 0, 0, 0, msk), Close: 2},
		{Start: at(21, 0), Close: 3},
	}}
	s := candlesServer(store, at(26, 12))
	resp, err := s.GetCandles(context.Background(), &securitiesreaderpb.GetCandlesRequest{Secid: "SBER", Range: securitiesreaderpb.CandleRange_CANDLE_RANGE_FIVE_YEARS})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_WEEK || len(resp.Candles) != 2 || resp.Candles[0].Close != 2 {
		t.Fatalf("resp = %+v", resp)
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

func TestGetDividends_DedupedFromDatabaseWithForecast(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}}
	store.dividendsLoaded = true
	store.dividends = []storage.Dividend{
		{RegistryCloseDate: "2027-07-20", Value: 44.53, Currency: "RUB", Forecast: true},
		{RegistryCloseDate: "2026-07-20", Value: 37.64, Currency: "RUB", DeclaredDate: "2026-04-21"},
		{RegistryCloseDate: "2026-07-20", Value: 37.64, Currency: "RUB"},
		{RegistryCloseDate: "2025-07-18", Value: 34.84, Currency: "RUB"},
	}
	s := candlesServer(store, at(26, 12))

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
	if store.dividendReads != 2 {
		t.Errorf("database reads=%d, want 2", store.dividendReads)
	}
}

func TestGetDividends_BondsSkipDividendQuery(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SU26238RMFS4": {"TQOB"}}}
	s := candlesServer(store, at(26, 12))
	resp, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "SU26238RMFS4"})
	if err != nil || len(resp.Dividends) != 0 || store.dividendReads != 0 {
		t.Errorf("resp=%v err=%v calls=%d", resp, err, store.dividendReads)
	}
}

func TestGetDividends_Errors(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}}
	s := candlesServer(store, at(26, 12))

	if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "NOPE"}); status.Code(err) != codes.NotFound {
		t.Errorf("unknown: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "SBER"}); status.Code(err) != codes.Unavailable {
			t.Errorf("source down: %v", err)
		}
	}
	if store.dividendReads != 2 {
		t.Errorf("database reads = %d", store.dividendReads)
	}
	if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty: %v", err)
	}
}

func TestGetSecurityInfoReflectsDatabaseUpdates(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, metadata: storage.SecurityMetadata{Issuer: "Old"}}
	s := newTestServer(store)
	req := &securitiesreaderpb.GetSecurityInfoRequest{Secid: "SBER"}
	first, err := s.GetSecurityInfo(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	store.metadata.Issuer = "New"
	second, err := s.GetSecurityInfo(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fields[0].Value != "Old" || second.Fields[0].Value != "New" {
		t.Fatalf("first=%v second=%v", first, second)
	}
}

func TestGetDividendsLoadedEmptySnapshot(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, dividendsLoaded: true}
	resp, err := newTestServer(store).GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "SBER"})
	if err != nil || len(resp.GetDividends()) != 0 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}

func TestStoredMetadataDatabaseErrors(t *testing.T) {
	store := &fakeStore{boards: map[string][]string{"SBER": {"TQBR"}}, metadataErr: errors.New("database down"), dividendsErr: errors.New("database down")}
	s := newTestServer(store)
	if _, err := s.GetSecurityInfo(context.Background(), &securitiesreaderpb.GetSecurityInfoRequest{Secid: "SBER"}); status.Code(err) != codes.Internal {
		t.Fatalf("info err=%v", err)
	}
	if _, err := s.GetDividends(context.Background(), &securitiesreaderpb.GetDividendsRequest{Secid: "SBER"}); status.Code(err) != codes.Internal {
		t.Fatalf("dividends err=%v", err)
	}
}
