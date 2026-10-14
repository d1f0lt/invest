package moex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
