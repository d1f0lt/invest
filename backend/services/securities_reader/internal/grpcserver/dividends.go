package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/securities_reader/internal/storage"
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
	raw, loaded, err := s.Store.Dividends(ctx, secid)
	if err != nil {
		s.Log.Error("failed to load dividends", "secid", secid, "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if !loaded {
		return nil, status.Error(codes.Unavailable, "dividends have not been loaded yet")
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
	resp.Dividends = out
	return resp, nil
}

func onlyBonds(boards []string) bool {
	for _, b := range boards {
		if !storage.IsBondBoard(b) {
			return false
		}
	}
	return true
}
