package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	"invest/backend/services/gateway/internal/upstream"
)

type historyClient struct {
	portfoliopb.PortfolioServiceClient
	got *portfoliopb.GetValueHistoryRequest
}

func (c *historyClient) GetValueHistory(_ context.Context, in *portfoliopb.GetValueHistoryRequest, _ ...grpc.CallOption) (*portfoliopb.ValueHistory, error) {
	c.got = in
	day := time.Date(2026, 9, 1, 21, 0, 0, 0, time.UTC)
	return &portfoliopb.ValueHistory{Points: []*portfoliopb.ValuePoint{
		{Date: timestamppb.New(day), Value: 1000, NetDeposits: 900},
	}}, nil
}

func doHistory(t *testing.T, client *historyClient, query string) *httptest.ResponseRecorder {
	t.Helper()
	h := &Handlers{
		Upstream:        &upstream.Clients{Portfolio: client},
		UpstreamTimeout: time.Second,
		Log:             testLogger(),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolios/p1/history"+query, nil)
	req.SetPathValue("id", "p1")
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "u1"))
	rec := httptest.NewRecorder()
	h.handleGetValueHistory(rec, req)
	return rec
}

func TestHandleGetValueHistory(t *testing.T) {
	client := &historyClient{}
	rec := doHistory(t, client, "?range=Month")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if client.got.GetPortfolioId() != "p1" || client.got.GetRange() != "month" {
		t.Errorf("upstream got %+v", client.got)
	}
	var resp valueHistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Range != "month" || len(resp.Points) != 1 || resp.Points[0].Value != 1000 || resp.Points[0].NetDeposits != 900 {
		t.Errorf("response = %s", rec.Body)
	}

	if rec := doHistory(t, client, ""); rec.Code != http.StatusOK || client.got.GetRange() != "all" {
		t.Errorf("default range: %d %q", rec.Code, client.got.GetRange())
	}
	if rec := doHistory(t, &historyClient{}, "?range=day"); rec.Code != http.StatusBadRequest {
		t.Errorf("range=day: status %d, want 400", rec.Code)
	}
}
