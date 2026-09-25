package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	securitiesreaderpb "invest/backend/services/gateway/internal/securitiesreaderpb"
)

type fakeSecuritiesSearch struct {
	fakeSecuritiesReader
	gotSearch  *securitiesreaderpb.SearchSecuritiesRequest
	searchResp *securitiesreaderpb.SearchSecuritiesResponse
	searchErr  error
}

func (f *fakeSecuritiesSearch) SearchSecurities(_ context.Context, in *securitiesreaderpb.SearchSecuritiesRequest, _ ...grpc.CallOption) (*securitiesreaderpb.SearchSecuritiesResponse, error) {
	f.gotSearch = in
	return f.searchResp, f.searchErr
}

func serveSearch(f *fakeSecuritiesSearch, target string) *httptest.ResponseRecorder {
	h := candlesHandlers(nil)
	h.Upstream.SecuritiesReader = f
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/securities", h.handleSearchSecurities)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandleSearchSecurities_MapsRequestAndResponse(t *testing.T) {
	name := "Сбербанк"
	f := &fakeSecuritiesSearch{searchResp: &securitiesreaderpb.SearchSecuritiesResponse{
		Securities: []*securitiesreaderpb.PriceView{{Secid: "SBER", Board: "TQBR", ShortName: &name, CollectedAt: timestamppb.Now()}},
	}}

	rec := serveSearch(f, "/api/v1/securities?q=%D1%81%D0%B1%D0%B5%D1%80&limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.gotSearch.GetQuery() != "сбер" || f.gotSearch.GetLimit() != 5 {
		t.Errorf("request = %+v", f.gotSearch)
	}
	var body struct {
		Count      int         `json:"count"`
		Securities []priceView `json:"securities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Count != 1 || body.Securities[0].SecID != "SBER" || *body.Securities[0].ShortName != name {
		t.Errorf("body = %+v", body)
	}
}

func TestHandleSearchSecurities_BadRequest(t *testing.T) {
	for _, target := range []string{
		"/api/v1/securities",
		"/api/v1/securities?q=%20%20",
		"/api/v1/securities?q=sber&limit=abc",
		"/api/v1/securities?q=sber&limit=0",
	} {
		f := &fakeSecuritiesSearch{}
		rec := serveSearch(f, target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
		}
		if f.gotSearch != nil {
			t.Errorf("%s: upstream should not be called", target)
		}
	}
}

func TestHandleSearchSecurities_UpstreamInvalidArgument(t *testing.T) {
	f := &fakeSecuritiesSearch{searchErr: status.Error(codes.InvalidArgument, "query must be at most 64 characters")}
	rec := serveSearch(f, "/api/v1/securities?q=x")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
