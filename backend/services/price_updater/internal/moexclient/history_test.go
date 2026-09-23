package moexclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchBoardHistoryPaginatesAndSkipsUntraded(t *testing.T) {
	pages := map[string]string{
		"0": `{"history":{"columns":["BOARDID","TRADEDATE","SECID","OPEN","LOW","HIGH","CLOSE","VOLUME","VALUE"],
			"data":[["TQBR","2026-09-23","SBER",300.0,298.5,305.0,304.2,1000,304200.0],
			        ["TQBR","2026-09-23","ILLIQ",null,null,null,null,0,0]]},
			"history.cursor":{"columns":["INDEX","TOTAL","PAGESIZE"],"data":[[0,3,2]]}}`,
		"2": `{"history":{"columns":["BOARDID","TRADEDATE","SECID","OPEN","LOW","HIGH","CLOSE","VOLUME","VALUE"],
			"data":[["TQBR","2026-09-23","GAZP",120.0,119.0,121.0,120.5,500,60250.0]]},
			"history.cursor":{"columns":["INDEX","TOTAL","PAGESIZE"],"data":[[2,3,2]]}}`,
	}
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.URL.Path; got != "/history/engines/stock/markets/shares/boards/TQBR/securities.json" {
			t.Errorf("unexpected path %s", got)
		}
		if got := r.URL.Query().Get("date"); got != "2026-09-23" {
			t.Errorf("date = %s", got)
		}
		body, ok := pages[r.URL.Query().Get("start")]
		if !ok {
			t.Errorf("unexpected start %q", r.URL.Query().Get("start"))
			body = `{"history":{"columns":[],"data":[]}}`
		}
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := New(srv.URL, 0)
	got, err := c.FetchBoardHistory(context.Background(), "TQBR", time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candles, want 2 (ILLIQ had no trades): %+v", len(got), got)
	}
	sber := got[0]
	if sber.SecID != "SBER" || sber.Open != 300 || sber.High != 305 || sber.Low != 298.5 || sber.Close != 304.2 {
		t.Errorf("SBER = %+v", sber)
	}
	if sber.Volume == nil || *sber.Volume != 1000 {
		t.Errorf("SBER volume = %v", sber.Volume)
	}
	if !sber.TradeDate.Equal(time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("TradeDate = %v", sber.TradeDate)
	}
}

func TestFetchBoardHistoryBondsMarket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/history/engines/stock/markets/bonds/boards/TQOB/securities.json" {
			t.Errorf("unexpected path %s", got)
		}
		_, _ = fmt.Fprint(w, `{"history":{"columns":["BOARDID","TRADEDATE","SECID","OPEN","LOW","HIGH","CLOSE"],"data":[]}}`)
	}))
	defer srv.Close()

	got, err := New(srv.URL, 0).FetchBoardHistory(context.Background(), "TQOB", time.Now())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func f(v float64) *float64 { return &v }
func i(v int64) *int64     { return &v }

func TestMergeSessionsPrefersTotalRow(t *testing.T) {
	rows := []historyRow{
		{secID: "SBER", tradeDate: "2026-09-23", session: 1, open: f(300), high: f(305), low: f(299), close: f(304), volume: i(10)},
		{secID: "SBER", tradeDate: "2026-09-23", session: 3, open: f(300), high: f(306), low: f(298), close: f(303), volume: i(15)},
		{secID: "SBER", tradeDate: "2026-09-23", session: 2, open: f(304), high: f(306), low: f(298), close: f(303), volume: i(5)},
	}
	got := mergeSessions(rows, "TQBR")
	if len(got) != 1 {
		t.Fatalf("got %d candles", len(got))
	}
	c := got[0]
	if c.Open != 300 || c.High != 306 || c.Low != 298 || c.Close != 303 || *c.Volume != 15 {
		t.Errorf("candle = %+v (volume %d)", c, *c.Volume)
	}
	if c.BoardID != "TQBR" {
		t.Errorf("board fallback = %q", c.BoardID)
	}
}

func TestMergeSessionsCombinesWithoutTotal(t *testing.T) {
	rows := []historyRow{
		{secID: "SBER", tradeDate: "2026-09-23", session: 2, open: f(304), high: f(307), low: f(302), close: f(306), volume: i(5)},
		{secID: "SBER", tradeDate: "2026-09-23", session: 1, open: f(300), high: f(305), low: f(299), close: f(304), volume: i(10)},
	}
	c := mergeSessions(rows, "TQBR")[0]
	if c.Open != 300 || c.Close != 306 || c.High != 307 || c.Low != 299 || *c.Volume != 15 {
		t.Errorf("candle = %+v", c)
	}
}
