package moexclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchIntradayCandlesPagination(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/engines/stock/markets/bonds/boards/TQOB/securities/BOND/candles.json" || r.URL.Query().Get("interval") != "10" || r.URL.Query().Get("from") != "2026-10-14" || r.URL.Query().Get("till") != "2026-10-14" {
			t.Errorf("request = %s", r.URL)
		}
		switch r.URL.Query().Get("start") {
		case "0":
			fmt.Fprint(w, `{"candles":{"columns":["begin","close","open","low","high","volume","value"],"data":[["2026-10-14 10:00:00",102,100,99,103,12,1200]]}}`)
		case "1":
			fmt.Fprint(w, `{"candles":{"columns":[],"data":[]}}`)
		default:
			t.Errorf("unexpected page: %s", r.URL)
			http.Error(w, "page", 500)
		}
	}))
	defer server.Close()
	loc := time.FixedZone("MSK", 10800)
	day := time.Date(2026, 10, 14, 0, 0, 0, 0, loc)
	got, err := New(server.URL, time.Second).FetchIntradayCandles(context.Background(), "BOND", "TQOB", day, loc)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(got) != 1 || got[0].Close != 102 || got[0].Volume == nil || *got[0].Volume != 12 || !got[0].TradeDate.Equal(day.Add(10*time.Hour)) {
		t.Fatalf("calls=%d candles=%+v", calls, got)
	}
}

func TestFetchIntradayCandlesRejectsBrokenResponse(t *testing.T) {
	for _, body := range []string{`{"candles":{"columns":["begin"],"data":[["bad"]]}}`, `{"candles":{"columns":["begin","open","high","low","close"],"data":[["bad",1,2,1,2]]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := New(server.URL, time.Second).FetchIntradayCandles(context.Background(), "SBER", "TQBR", time.Now(), time.UTC)
		server.Close()
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
