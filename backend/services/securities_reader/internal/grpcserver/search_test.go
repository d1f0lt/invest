package grpcserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/securities_reader/internal/storage"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

func (f *fakeStore) SearchSecurities(_ context.Context, query string, limit int) ([]storage.PriceView, error) {
	f.gotQuery, f.gotLimit = query, limit
	return f.found, nil
}

func TestSearchSecurities_NormalizesQueryAndDefaultsLimit(t *testing.T) {
	name := "Сбербанк"
	store := &fakeStore{found: []storage.PriceView{{SecID: "SBER", Board: "TQBR", ShortName: &name, CollectedAt: time.Now()}}}
	s := newTestServer(store)

	resp, err := s.SearchSecurities(context.Background(), &securitiesreaderpb.SearchSecuritiesRequest{Query: "  ЁЛКИ   Палки "})
	if err != nil {
		t.Fatalf("SearchSecurities: %v", err)
	}
	if store.gotQuery != "елки палки" {
		t.Errorf("query = %q, want %q", store.gotQuery, "елки палки")
	}
	if store.gotLimit != defaultSearchLimit {
		t.Errorf("limit = %d, want %d", store.gotLimit, defaultSearchLimit)
	}
	if len(resp.Securities) != 1 || resp.Securities[0].GetSecid() != "SBER" || resp.Securities[0].GetShortName() != name {
		t.Errorf("unexpected securities: %+v", resp.Securities)
	}
}

func TestSearchSecurities_CapsLimit(t *testing.T) {
	store := &fakeStore{}
	s := newTestServer(store)

	if _, err := s.SearchSecurities(context.Background(), &securitiesreaderpb.SearchSecuritiesRequest{Query: "sber", Limit: 1000}); err != nil {
		t.Fatalf("SearchSecurities: %v", err)
	}
	if store.gotLimit != maxSearchLimit {
		t.Errorf("limit = %d, want %d", store.gotLimit, maxSearchLimit)
	}
}

func TestSearchSecurities_InvalidArgument(t *testing.T) {
	cases := map[string]*securitiesreaderpb.SearchSecuritiesRequest{
		"empty":          {Query: "   "},
		"too long":       {Query: strings.Repeat("я", maxQueryLen+1)},
		"negative limit": {Query: "sber", Limit: -1},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			_, err := newTestServer(store).SearchSecurities(context.Background(), req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument (err %v)", status.Code(err), err)
			}
			if store.gotQuery != "" {
				t.Errorf("store should not be queried, got %q", store.gotQuery)
			}
		})
	}
}
