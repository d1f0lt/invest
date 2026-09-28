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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	"invest/backend/services/gateway/internal/upstream"
)

type renameClient struct {
	portfoliopb.PortfolioServiceClient
	got *portfoliopb.UpdatePortfolioRequest
	err error
}

func (c *renameClient) UpdatePortfolio(_ context.Context, in *portfoliopb.UpdatePortfolioRequest, _ ...grpc.CallOption) (*portfoliopb.Portfolio, error) {
	c.got = in
	if c.err != nil {
		return nil, c.err
	}
	return &portfoliopb.Portfolio{Id: in.GetId(), Name: in.GetName()}, nil
}

func doRename(t *testing.T, client *renameClient, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := &Handlers{
		Upstream:        &upstream.Clients{Portfolio: client},
		UpstreamTimeout: time.Second,
		Log:             testLogger(),
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/portfolios/p1", strings.NewReader(body))
	req.SetPathValue("id", "p1")
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "u1"))
	rec := httptest.NewRecorder()
	h.handleUpdatePortfolio(rec, req)
	return rec
}

func TestHandleUpdatePortfolio_PassesIDAndName(t *testing.T) {
	client := &renameClient{}
	rec := doRename(t, client, `{"name":"ИИС"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if client.got.GetId() != "p1" || client.got.GetName() != "ИИС" {
		t.Errorf("upstream got %+v", client.got)
	}
	var resp portfolioResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Name != "ИИС" {
		t.Errorf("response = %s (err %v)", rec.Body, err)
	}
}

func TestHandleUpdatePortfolio_InvalidNameIs400(t *testing.T) {
	client := &renameClient{err: status.Error(codes.InvalidArgument, "name is required")}
	if rec := doRename(t, client, `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
