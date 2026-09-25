package dohod

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestParseRealPage(t *testing.T) {
	page, err := os.ReadFile("testdata/sber.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(string(page))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 20 {
		t.Fatalf("got %d dividends, want the full history", len(got))
	}
	first := got[0]
	if first.RegistryCloseDate != "2027-07-20" || first.Value != 44.53 || !first.Forecast || first.DeclaredDate != "" {
		t.Errorf("forecast row = %+v", first)
	}
	second := got[1]
	if second.RegistryCloseDate != "2026-07-20" || second.Value != 37.64 || second.Forecast || second.DeclaredDate != "2026-04-21" {
		t.Errorf("latest paid row = %+v", second)
	}
	last := got[len(got)-1]
	if last.RegistryCloseDate != "2000-06-12" || last.Value != 0.0328 {
		t.Errorf("oldest row = %+v", last)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].RegistryCloseDate < got[i].RegistryCloseDate {
			t.Fatalf("not sorted newest first at %d", i)
		}
	}
}

func TestParseMissingTable(t *testing.T) {
	if _, err := Parse(`<html><body><p>Нет данных</p></body></html>`); !errors.Is(err, ErrTableNotFound) {
		t.Errorf("err = %v, want ErrTableNotFound", err)
	}
}

func TestParseEmptyTable(t *testing.T) {
	got, err := Parse(`<p class="table-title">Все выплаты</p><table class="content-table"><tr><th>Дата объявления дивиденда</th><th>Дата закрытия реестра</th><th>Год</th><th>Дивиденд</th></tr></table>`)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v err %v", got, err)
	}
}

func TestClient(t *testing.T) {
	page, _ := os.ReadFile("testdata/sber.html")
	var gotPath, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA = r.URL.Path, r.UserAgent()
		switch r.URL.Path {
		case "/ik/analytics/dividend/sber":
			w.Write(page)
		case "/ik/analytics/dividend/down":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL+"/ik/analytics/dividend/", time.Second)

	got, err := c.Dividends(context.Background(), "SBER")
	if err != nil || len(got) == 0 {
		t.Fatalf("got %d err %v", len(got), err)
	}
	if gotPath != "/ik/analytics/dividend/sber" || gotUA == "" {
		t.Errorf("path %q ua %q", gotPath, gotUA)
	}
	if got, err := c.Dividends(context.Background(), "NOPE"); err != nil || got != nil {
		t.Errorf("404: got %v err %v, want empty", got, err)
	}
	if _, err := c.Dividends(context.Background(), "DOWN"); err == nil {
		t.Error("503: want error")
	}
}
