package moex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var msk = time.FixedZone("MSK", 3*3600)

func TestIntradayCandles(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Write([]byte(`{"candles": {"columns": ["open", "close", "high", "low", "value", "volume", "begin", "end"], "data": [
			[279.1, 279.5, 279.8, 279.0, 1.5e8, 540000, "2026-09-25 10:00:00", "2026-09-25 10:09:59"],
			[279.5, 278.9, 279.6, 278.8, 9.1e7, 320000, "2026-09-25 10:10:00", "2026-09-25 10:19:59"]
		]}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, time.Second)
	got, err := c.IntradayCandles(context.Background(), "SBER", "TQBR", time.Date(2026, 9, 25, 0, 0, 0, 0, msk), msk)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/engines/stock/markets/shares/boards/TQBR/securities/SBER/candles.json" {
		t.Errorf("path = %s", gotPath)
	}
	if gotQuery == "" || !contains(gotQuery, "interval=10") || !contains(gotQuery, "from=2026-09-25") || !contains(gotQuery, "till=2026-09-25") {
		t.Errorf("query = %s", gotQuery)
	}
	if len(got) != 2 || got[1].Close != 278.9 || !got[1].Begin.Equal(time.Date(2026, 9, 25, 10, 10, 0, 0, msk)) {
		t.Errorf("candles = %+v", got)
	}
}

func TestIntradayCandles_BondsMarket(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"candles": {"columns": ["open", "close", "high", "low", "begin"], "data": []}}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL, time.Second).IntradayCandles(context.Background(), "SU26238RMFS4", "TQOB", time.Now(), msk); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/engines/stock/markets/bonds/boards/TQOB/securities/SU26238RMFS4/candles.json" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestDescriptionAndEmitter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/securities/SBER.json":
			w.Write([]byte(`{"description": {"columns": ["name", "title", "value", "type", "sort_order", "is_hidden", "precision"], "data": [
				["SECID", "Код ценной бумаги", "SBER", "string", 1, 0, null],
				["FACEVALUE", "Номинальная стоимость", "3", "number", 10, 0, 2],
				["TYPE", "Тип бумаги", "common_share", "string", 30, 1, null]
			]}}`))
		case "/securities.json":
			w.Write([]byte(`{"securities": {"columns": ["secid", "emitent_title"], "data": [
				["SBERP", "ПАО Сбербанк"], ["SBER", "Публичное акционерное общество \"Сбербанк России\""]
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second)

	desc, err := c.Description(context.Background(), "SBER")
	if err != nil {
		t.Fatal(err)
	}
	if len(desc) != 3 || desc[1].Name != "FACEVALUE" || desc[1].Value != "3" || !desc[2].Hidden || desc[0].Hidden {
		t.Errorf("desc = %+v", desc)
	}

	title, err := c.EmitterTitle(context.Background(), "SBER")
	if err != nil {
		t.Fatal(err)
	}
	if title != `Публичное акционерное общество "Сбербанк России"` {
		t.Errorf("title = %q", title)
	}
}

func TestNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, time.Second).Description(context.Background(), "SBER"); err == nil {
		t.Fatal("want error")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
