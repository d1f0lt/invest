package moexclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sample = `{
  "securities": {
    "columns": ["SECID","BOARDID","SHORTNAME","SECNAME","ISIN","CURRENCYID","LOTSIZE","DECIMALS"],
    "data": [
      ["SBER","TQBR","Сбербанк","Сбербанк России ПАО ао","RU0009029540","SUR",10,2],
      ["GAZP","TQBR","ГАЗПРОМ ао","Газпром ПАО ао","RU0007661625","SUR",10,2],
      ["NOMATCH","TQBR","НетКотировки","Тест без marketdata","RU000TEST0001","SUR",1,2]
    ]
  },
  "marketdata": {
    "columns": ["SECID","BOARDID","LAST","OPEN","HIGH","LOW","VALTODAY","VOLTODAY","TRADINGSTATUS","UPDATETIME"],
    "data": [
      ["SBER","TQBR",289.5,285.0,290.1,284.8,1234567.8,42000,"T","18:44:59"],
      ["GAZP","TQBR",null,150.0,null,null,null,null,"T",null]
    ]
  }
}`

func TestFetchBoardParsesSample(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/engines/stock/markets/shares/boards/TQBR/securities.json" {
			t.Errorf("unexpected path: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sample))
	}))
	defer srv.Close()

	c := New(srv.URL, 0)
	snap, err := c.FetchBoard(context.Background(), "TQBR")
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if snap.BoardID != "TQBR" {
		t.Errorf("BoardID = %q, want TQBR", snap.BoardID)
	}
	if len(snap.Securities) != 3 {
		t.Fatalf("got %d securities, want 3", len(snap.Securities))
	}
	if len(snap.Quotes) != 2 {
		t.Fatalf("got %d quotes, want 2", len(snap.Quotes))
	}

	sber, ok := snap.Quotes["SBER"]
	if !ok {
		t.Fatal("missing SBER quote")
	}
	if sber.Last == nil || *sber.Last != 289.5 {
		t.Errorf("SBER.Last = %v, want 289.5", sber.Last)
	}
	if sber.UpdateTime == nil || *sber.UpdateTime != "18:44:59" {
		t.Errorf("SBER.UpdateTime = %v, want 18:44:59", sber.UpdateTime)
	}

	gazp, ok := snap.Quotes["GAZP"]
	if !ok {
		t.Fatal("missing GAZP quote")
	}
	if gazp.Last != nil {
		t.Errorf("GAZP.Last = %v, want nil (market closed / no trades)", *gazp.Last)
	}
	if gazp.Open == nil || *gazp.Open != 150.0 {
		t.Errorf("GAZP.Open = %v, want 150.0", gazp.Open)
	}

	if _, hasQuote := snap.Quotes["NOMATCH"]; hasQuote {
		t.Errorf("NOMATCH should have no quote")
	}
	found := false
	for _, s := range snap.Securities {
		if s.SecID == "NOMATCH" {
			found = true
		}
	}
	if !found {
		t.Errorf("NOMATCH should still be present in Securities")
	}
}

func TestFetchBoardNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(srv.URL, 0)
	if _, err := c.FetchBoard(context.Background(), "TQBR"); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestFetchBoardBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := New(srv.URL, 0)
	if _, err := c.FetchBoard(context.Background(), "TQBR"); err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestSampleIsValidJSON(t *testing.T) {
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(sample), &v); err != nil {
		t.Fatalf("sample fixture is not valid JSON: %v", err)
	}
}

const bondSample = `{
  "securities": {
    "columns": ["SECID","BOARDID","SHORTNAME","SECNAME","ISIN","CURRENCYID","LOTSIZE","DECIMALS","FACEVALUE","PREVPRICE"],
    "data": [
      ["SU26254RMFS1","TQOB","ОФЗ 26254","ОФЗ-ПД 26254 03/10/40","RU000A10D533","SUR",1,4,1000,88.4],
      ["SU26238RMFS4","TQOB","ОФЗ 26238","ОФЗ-ПД 26238 15/05/41","RU000A1038V6","SUR",1,4,1000,60.1],
      ["SU26240RMFS0","TQOB","ОФЗ 26240","ОФЗ-ПД 26240 30/07/36","RU000A103BR0","SUR",1,4,1000,null]
    ]
  },
  "marketdata": {
    "columns": ["SECID","BOARDID","LAST","LCURRENTPRICE","OPEN","HIGH","LOW","VALTODAY","VOLTODAY","TRADINGSTATUS","UPDATETIME"],
    "data": [
      ["SU26254RMFS1","TQOB",88.5,88.45,88.1,88.6,88.0,1000,10,"T","18:39:59"],
      ["SU26238RMFS4","TQOB",null,60.3,null,null,null,null,null,"T",null],
      ["SU26240RMFS0","TQOB",null,null,null,null,null,null,null,"T",null]
    ]
  }
}`

func TestFetchBoard_Bonds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/engines/stock/markets/bonds/boards/TQOB/securities.json" {
			t.Errorf("unexpected path: %s", got)
		}
		_, _ = w.Write([]byte(bondSample))
	}))
	defer srv.Close()

	snap, err := New(srv.URL, 0).FetchBoard(context.Background(), "TQOB")
	if err != nil {
		t.Fatal(err)
	}
	sec := snap.Securities[0]
	if !sec.PriceInPercent || sec.FaceValue == nil || *sec.FaceValue != 1000 {
		t.Errorf("bond security = %+v", sec)
	}
	if q := snap.Quotes["SU26254RMFS1"]; q.Last == nil || *q.Last != 88.5 {
		t.Errorf("traded bond: LAST must be used, got %+v", q.Last)
	}
	if q := snap.Quotes["SU26238RMFS4"]; q.Last == nil || *q.Last != 60.3 {
		t.Errorf("untraded bond: must fall back to LCURRENTPRICE, got %+v", q.Last)
	}
	if q := snap.Quotes["SU26240RMFS0"]; q.Last != nil {
		t.Errorf("no price at all: want nil, got %v", *q.Last)
	}
}

func TestMarketFor(t *testing.T) {
	for board, want := range map[string]string{"TQBR": "shares", "TQTF": "shares", "TQOB": "bonds", "TQCB": "bonds"} {
		if got := MarketFor(board); got != want {
			t.Errorf("MarketFor(%s) = %s, want %s", board, got, want)
		}
	}
}
