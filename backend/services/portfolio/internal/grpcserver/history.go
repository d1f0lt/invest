package grpcserver

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/portfolio/internal/pnl"
	portfoliopb "invest/backend/services/portfolio/proto"
)

const maxDailyPoints = 400

const closesLookback = 20 * 24 * time.Hour

var moscow = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Moscow"); err == nil {
		return loc
	}
	return time.FixedZone("MSK", 3*60*60)
}()

func (s *Server) location() *time.Location {
	if s.Loc != nil {
		return s.Loc
	}
	return moscow
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) GetValueHistory(ctx context.Context, req *portfoliopb.GetValueHistoryRequest) (*portfoliopb.ValueHistory, error) {
	rangeName := strings.ToLower(strings.TrimSpace(req.GetRange()))
	now := s.now()
	var from time.Time
	switch rangeName {
	case "", "all":
	case "week":
		from = now.AddDate(0, 0, -7)
	case "month":
		from = now.AddDate(0, -1, 0)
	case "year":
		from = now.AddDate(-1, 0, 0)
	default:
		return nil, status.Error(codes.InvalidArgument, "range must be one of: week, month, year, all")
	}

	b, err := s.loadBook(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}

	var first time.Time
	for _, l := range b.ledgers {
		for _, t := range l.trades {
			if first.IsZero() || t.ExecutedAt.Before(first) {
				first = t.ExecutedAt
			}
		}
		for _, c := range l.cash {
			if first.IsZero() || c.OccurredAt.Before(first) {
				first = c.OccurredAt
			}
		}
	}
	if first.IsZero() {
		return &portfoliopb.ValueHistory{}, nil
	}
	if from.IsZero() || from.Before(first) {
		from = first
	}

	loc := s.location()
	days := pnl.HistoryDays(from, now, loc, maxDailyPoints)
	closes, err := s.Store.DailyCloses(ctx, b.instruments, days[0].Add(-closesLookback))
	if err != nil {
		s.Log.Error("daily closes", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	byKey := make(map[string][]pnl.DailyClose, len(closes))
	for key, list := range closes {
		converted := make([]pnl.DailyClose, len(list))
		for i, c := range list {
			converted[i] = pnl.DailyClose{Day: c.Day, Close: c.Close}
		}
		byKey[key] = converted
	}

	histories := make([][]pnl.ValuePoint, 0, len(b.ledgers))
	for _, l := range b.ledgers {
		histories = append(histories, pnl.ValueHistory(l.trades, l.cash, byKey, b.prices, days, now))
	}
	points := pnl.MergeValueHistory(histories)
	out := &portfoliopb.ValueHistory{Points: make([]*portfoliopb.ValuePoint, 0, len(points))}
	for _, p := range points {
		out.Points = append(out.Points, &portfoliopb.ValuePoint{
			Date:        timestamppb.New(p.Day),
			Value:       p.Value,
			NetDeposits: p.NetDeposits,
		})
	}
	return out, nil
}
