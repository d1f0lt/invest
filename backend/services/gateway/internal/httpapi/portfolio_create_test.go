package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	"invest/backend/services/gateway/internal/upstream"
)

type createClient struct {
	portfoliopb.PortfolioServiceClient
	got *portfoliopb.CreatePortfolioRequest
}

func (c *createClient) CreatePortfolio(_ context.Context, in *portfoliopb.CreatePortfolioRequest, _ ...grpc.CallOption) (*portfoliopb.Portfolio, error) {
	c.got = in
	return &portfoliopb.Portfolio{Id: "c1", Name: in.GetName(), MemberIds: in.GetMemberIds()}, nil
}

func doCreate(t *testing.T, client *createClient, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := &Handlers{
		Upstream:        &upstream.Clients{Portfolio: client},
		UpstreamTimeout: time.Second,
		Log:             testLogger(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolios", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "u1"))
	rec := httptest.NewRecorder()
	h.handleCreatePortfolio(rec, req)
	return rec
}

func TestHandleCreatePortfolio_Composite(t *testing.T) {
	client := &createClient{}
	rec := doCreate(t, client, `{"name":"Всё","member_ids":["a","b"]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if client.got.GetName() != "Всё" || strings.Join(client.got.GetMemberIds(), ",") != "a,b" {
		t.Errorf("upstream got %+v", client.got)
	}
	var resp portfolioResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || strings.Join(resp.MemberIDs, ",") != "a,b" {
		t.Errorf("response = %s (err %v)", rec.Body, err)
	}
}

func TestHandleCreatePortfolio_OrdinaryHasNoMembers(t *testing.T) {
	client := &createClient{}
	rec := doCreate(t, client, `{"name":"ИИС"}`)

	if rec.Code != http.StatusCreated || len(client.got.GetMemberIds()) != 0 {
		t.Fatalf("status = %d, upstream got %+v", rec.Code, client.got)
	}
	if strings.Contains(rec.Body.String(), "member_ids") {
		t.Errorf("response = %s, want no member_ids", rec.Body)
	}
}
