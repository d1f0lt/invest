package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pricereaderpb "invest/backend/services/gateway/internal/pricereaderpb"
	"invest/backend/services/gateway/internal/upstream"
)

type fakePriceReader struct {
	pricereaderpb.PriceReaderServiceClient
	got  *pricereaderpb.GetCandlesRequest
	resp *pricereaderpb.GetCandlesResponse
	err  error
}

func (f *fakePriceReader) GetCandles(_ context.Context, in *pricereaderpb.GetCandlesRequest, _ ...grpc.CallOption) (*pricereaderpb.GetCandlesResponse, error) {
	f.got = in
	return f.resp, f.err
}

func candlesHandlers(f *fakePriceReader) *Handlers {
	return &Handlers{
		Upstream:        &upstream.Clients{PriceReader: f},
		UpstreamTimeout: time.Second,
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func serveCandles(h *Handlers, target string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/prices/{secid}/candles", h.handleGetCandles)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandleGetCandles_MapsRequestAndResponse(t *testing.T) {
	start := time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC)
	vol := int64(42)
	f := &fakePriceReader{resp: &pricereaderpb.GetCandlesResponse{
		Secid: "SBER", Board: "TQBR",
		Range:    pricereaderpb.CandleRange_CANDLE_RANGE_WEEK,
		Interval: pricereaderpb.CandleInterval_CANDLE_INTERVAL_HOUR,
		Candles:  []*pricereaderpb.Candle{{Start: timestamppb.New(start), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: &vol}},
	}}

	rec := serveCandles(candlesHandlers(f), "/api/v1/prices/SBER/candles?range=Week&board=TQBR")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.got.GetSecid() != "SBER" || f.got.GetBoard() != "TQBR" || f.got.GetRange() != pricereaderpb.CandleRange_CANDLE_RANGE_WEEK {
		t.Errorf("upstream request = %+v", f.got)
	}

	var body candlesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Range != "week" || body.Interval != "hour" || body.Count != 1 {
		t.Errorf("body = %+v", body)
	}
	c := body.Candles[0]
	if !c.Start.Equal(start) || c.Close != 1.5 || c.Volume == nil || *c.Volume != 42 || c.Value != nil {
		t.Errorf("candle = %+v", c)
	}
}

func TestHandleGetCandles_DefaultsToDay(t *testing.T) {
	f := &fakePriceReader{resp: &pricereaderpb.GetCandlesResponse{}}
	rec := serveCandles(candlesHandlers(f), "/api/v1/prices/SBER/candles")
	if rec.Code != http.StatusOK || f.got.GetRange() != pricereaderpb.CandleRange_CANDLE_RANGE_DAY {
		t.Errorf("status %d, range %v", rec.Code, f.got.GetRange())
	}
	if want := `"candles":[]`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("empty result must serialize as [], got %s", rec.Body)
	}
}

func TestHandleGetCandles_BadRange(t *testing.T) {
	f := &fakePriceReader{}
	rec := serveCandles(candlesHandlers(f), "/api/v1/prices/SBER/candles?range=decade")
	if rec.Code != http.StatusBadRequest || f.got != nil {
		t.Errorf("status %d, upstream called: %v", rec.Code, f.got != nil)
	}
}

func TestHandleGetCandles_UpstreamNotFound(t *testing.T) {
	f := &fakePriceReader{err: status.Error(codes.NotFound, "unknown security NOPE")}
	rec := serveCandles(candlesHandlers(f), "/api/v1/prices/NOPE/candles?range=day")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d", rec.Code)
	}
}
