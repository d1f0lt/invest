package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/securities_reader/internal/moex"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

func (s *Server) GetDividends(ctx context.Context, req *securitiesreaderpb.GetDividendsRequest) (*securitiesreaderpb.GetDividendsResponse, error) {
	secid := strings.ToUpper(strings.TrimSpace(req.GetSecid()))
	if secid == "" {
		return nil, status.Error(codes.InvalidArgument, "secid is required")
	}
	boards, err := s.Store.BoardsOf(ctx, secid)
	if err != nil {
		s.Log.Error("failed to load boards", "secid", secid, "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if len(boards) == 0 {
		return nil, status.Errorf(codes.NotFound, "unknown security %s", secid)
	}
	resp := &securitiesreaderpb.GetDividendsResponse{Secid: secid}
	if onlyBonds(boards) {
		return resp, nil
	}
	if s.DividendSource == nil {
		return nil, status.Error(codes.Unavailable, "dividends source is not configured")
	}

	s.caches()
	if cached, ok := s.dividends.get(secid, s.now()); ok {
		resp.Dividends = cached
		return resp, nil
	}

	raw, err := s.DividendSource.Dividends(ctx, secid)
	if err != nil {
		s.Log.Warn("dividends unavailable", "secid", secid, "error", err)
		return nil, status.Error(codes.Unavailable, "dividends are temporarily unavailable")
	}

	seen := make(map[string]bool, len(raw))
	out := make([]*securitiesreaderpb.Dividend, 0, len(raw))
	for _, d := range raw {
		key := d.RegistryCloseDate + "|" + d.Currency
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, &securitiesreaderpb.Dividend{
			RegistryCloseDate: d.RegistryCloseDate,
			Value:             d.Value,
			Currency:          d.Currency,
			Forecast:          d.Forecast,
			DeclaredDate:      d.DeclaredDate,
		})
	}
	s.Log.Info("dividends loaded", "secid", secid, "count", len(out))
	s.dividends.put(secid, out, s.now())
	resp.Dividends = out
	return resp, nil
}

func onlyBonds(boards []string) bool {
	for _, b := range boards {
		if moex.MarketFor(b) != "bonds" {
			return false
		}
	}
	return true
}
