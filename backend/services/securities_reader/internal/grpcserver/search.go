package grpcserver

import (
	"context"
	"strings"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
	maxQueryLen        = 64
)

func (s *Server) SearchSecurities(ctx context.Context, req *securitiesreaderpb.SearchSecuritiesRequest) (*securitiesreaderpb.SearchSecuritiesResponse, error) {
	query := normalizeQuery(req.GetQuery())
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	if utf8.RuneCountInString(query) > maxQueryLen {
		return nil, status.Errorf(codes.InvalidArgument, "query must be at most %d characters", maxQueryLen)
	}

	limit := int(req.GetLimit())
	switch {
	case limit < 0:
		return nil, status.Error(codes.InvalidArgument, "limit must not be negative")
	case limit == 0:
		limit = defaultSearchLimit
	case limit > maxSearchLimit:
		limit = maxSearchLimit
	}

	found, err := s.Store.SearchSecurities(ctx, query, limit)
	if err != nil {
		s.Log.Error("failed to search securities", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	out := make([]*securitiesreaderpb.PriceView, 0, len(found))
	for _, p := range found {
		out = append(out, toPriceViewPB(p))
	}
	return &securitiesreaderpb.SearchSecuritiesResponse{Securities: out}, nil
}

func normalizeQuery(q string) string {
	q = strings.Join(strings.Fields(q), " ")
	return strings.ReplaceAll(strings.ToLower(q), "ё", "е")
}
