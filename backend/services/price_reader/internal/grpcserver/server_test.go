package grpcserver

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/price_reader/internal/storage"
	pricereaderpb "invest/backend/services/price_reader/proto"
)

type fakeStore struct {
	all         []storage.PriceView
	byTicker    map[string]storage.PriceView
	lastQueried []string
}

func (f *fakeStore) AllLatestPrices(_ context.Context) ([]storage.PriceView, error) {
	return f.all, nil
}

func (f *fakeStore) LatestPricesByTickers(_ context.Context, tickers []string) ([]storage.PriceView, error) {
	f.lastQueried = tickers
	out := make([]storage.PriceView, 0, len(tickers))
	for _, t := range tickers {
		if v, ok := f.byTicker[t]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func newTestServer(store Store) *Server {
	return &Server{Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestGetPrices_EmptyTickersReturnsAll(t *testing.T) {
	store := &fakeStore{all: []storage.PriceView{{SecID: "SBER", Board: "TQBR", CollectedAt: time.Now()}}}
	s := newTestServer(store)

	resp, err := s.GetPrices(context.Background(), &pricereaderpb.GetPricesRequest{})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(resp.Prices) != 1 || resp.Prices[0].Secid != "SBER" {
		t.Errorf("unexpected prices: %+v", resp.Prices)
	}
}

func TestGetPrices_NormalizesTickers(t *testing.T) {
	store := &fakeStore{byTicker: map[string]storage.PriceView{
		"SBER": {SecID: "SBER", Board: "TQBR", CollectedAt: time.Now()},
	}}
	s := newTestServer(store)

	resp, err := s.GetPrices(context.Background(), &pricereaderpb.GetPricesRequest{Tickers: []string{" sber ", "SBER", "sber"}})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(store.lastQueried) != 1 || store.lastQueried[0] != "SBER" {
		t.Fatalf("tickers not normalized/deduped: %v", store.lastQueried)
	}
	if len(resp.Prices) != 1 {
		t.Errorf("unexpected prices: %+v", resp.Prices)
	}
}

func TestGetPrices_AllBlankTickersIsInvalidArgument(t *testing.T) {
	s := newTestServer(&fakeStore{})
	_, err := s.GetPrices(context.Background(), &pricereaderpb.GetPricesRequest{Tickers: []string{"  ", ""}})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}
