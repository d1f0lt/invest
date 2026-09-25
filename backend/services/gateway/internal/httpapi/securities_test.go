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

type fakeSecurityInfo struct {
	fakeSecuritiesReader
	gotInfo  *securitiesreaderpb.GetSecurityInfoRequest
	infoResp *securitiesreaderpb.GetSecurityInfoResponse
	infoErr  error
}

func (f *fakeSecurityInfo) GetSecurityInfo(_ context.Context, in *securitiesreaderpb.GetSecurityInfoRequest, _ ...grpc.CallOption) (*securitiesreaderpb.GetSecurityInfoResponse, error) {
	f.gotInfo = in
	return f.infoResp, f.infoErr
}

func serveInfo(f *fakeSecurityInfo, target string) *httptest.ResponseRecorder {
	h := candlesHandlers(nil)
	h.Upstream.SecuritiesReader = f
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/securities/{secid}/info", h.handleGetSecurityInfo)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandleGetSecurityInfo(t *testing.T) {
	unit := "SUR"
	f := &fakeSecurityInfo{infoResp: &securitiesreaderpb.GetSecurityInfoResponse{
		Secid: "SBER", Board: "TQBR",
		Fields: []*securitiesreaderpb.SecurityInfoField{
			{Name: "ISSUER", Title: "Эмитент", Value: "ПАО Сбербанк", Type: "text"},
			{Name: "FACEVALUE", Title: "Номинал", Value: "3", Type: "money", Unit: &unit},
		},
	}}
	rec := serveInfo(f, "/api/v1/securities/SBER/info?board=TQBR")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.gotInfo.GetSecid() != "SBER" || f.gotInfo.GetBoard() != "TQBR" {
		t.Errorf("request = %+v", f.gotInfo)
	}
	var body securityInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Fields) != 2 || body.Fields[1].Unit == nil || *body.Fields[1].Unit != "SUR" || body.Fields[0].Unit != nil {
		t.Errorf("body = %+v", body)
	}
}

func TestHandleGetSecurityInfo_NotFound(t *testing.T) {
	f := &fakeSecurityInfo{infoErr: status.Error(codes.NotFound, "unknown security NOPE")}
	if rec := serveInfo(f, "/api/v1/securities/NOPE/info"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

type fakeDividends struct {
	fakeSecuritiesReader
	gotDiv  *securitiesreaderpb.GetDividendsRequest
	divResp *securitiesreaderpb.GetDividendsResponse
	divErr  error
}

func (f *fakeDividends) GetDividends(_ context.Context, in *securitiesreaderpb.GetDividendsRequest, _ ...grpc.CallOption) (*securitiesreaderpb.GetDividendsResponse, error) {
	f.gotDiv = in
	return f.divResp, f.divErr
}

func serveDividends(f *fakeDividends, target string) *httptest.ResponseRecorder {
	h := candlesHandlers(nil)
	h.Upstream.SecuritiesReader = f
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/securities/{secid}/dividends", h.handleGetDividends)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandleGetDividends(t *testing.T) {
	f := &fakeDividends{divResp: &securitiesreaderpb.GetDividendsResponse{
		Secid: "SBER",
		Dividends: []*securitiesreaderpb.Dividend{
			{RegistryCloseDate: "2027-07-20", Value: 44.53, Currency: "RUB", Forecast: true},
			{RegistryCloseDate: "2026-07-20", Value: 37.64, Currency: "RUB", DeclaredDate: "2026-04-21"},
		},
	}}
	rec := serveDividends(f, "/api/v1/securities/SBER/dividends")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.gotDiv.GetSecid() != "SBER" {
		t.Errorf("request = %+v", f.gotDiv)
	}
	var body struct {
		Count     int            `json:"count"`
		Dividends []dividendView `json:"dividends"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 2 || !body.Dividends[0].Forecast || body.Dividends[0].DeclaredDate != nil ||
		body.Dividends[1].Value != 37.64 || body.Dividends[1].DeclaredDate == nil || *body.Dividends[1].DeclaredDate != "2026-04-21" {
		t.Errorf("body = %+v", body)
	}
}

func TestHandleGetDividends_Unavailable(t *testing.T) {
	f := &fakeDividends{divErr: status.Error(codes.Unavailable, "down")}
	if rec := serveDividends(f, "/api/v1/securities/SBER/dividends"); rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}
