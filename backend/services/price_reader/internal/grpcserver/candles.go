package grpcserver

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/price_reader/internal/storage"
	pricereaderpb "invest/backend/services/price_reader/proto"
)

var moscowFallback = time.FixedZone("MSK", 3*60*60)

func (s *Server) GetCandles(ctx context.Context, req *pricereaderpb.GetCandlesRequest) (*pricereaderpb.GetCandlesResponse, error) {
	secid := strings.ToUpper(strings.TrimSpace(req.GetSecid()))
	if secid == "" {
		return nil, status.Error(codes.InvalidArgument, "secid is required")
	}
	if req.GetRange() == pricereaderpb.CandleRange_CANDLE_RANGE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "range is required")
	}
	if _, ok := pricereaderpb.CandleRange_name[int32(req.GetRange())]; !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown range %d", req.GetRange())
	}

	board, err := s.resolveBoard(ctx, secid, strings.ToUpper(strings.TrimSpace(req.GetBoard())))
	if err != nil {
		return nil, err
	}

	candles, interval, err := s.loadCandles(ctx, secid, board, req.GetRange())
	if err != nil {
		s.Log.Error("failed to load candles", "secid", secid, "board", board, "range", req.GetRange().String(), "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	out := make([]*pricereaderpb.Candle, 0, len(candles))
	for _, c := range candles {
		out = append(out, &pricereaderpb.Candle{
			Start: timestamppb.New(c.Start),
			Open:  c.Open, High: c.High, Low: c.Low, Close: c.Close,
			Volume: c.Volume, Value: c.Value,
		})
	}
	return &pricereaderpb.GetCandlesResponse{
		Secid:    secid,
		Board:    board,
		Range:    req.GetRange(),
		Interval: interval,
		Candles:  out,
	}, nil
}

func (s *Server) resolveBoard(ctx context.Context, secid, board string) (string, error) {
	boards, err := s.Store.BoardsOf(ctx, secid)
	if err != nil {
		s.Log.Error("failed to load boards", "secid", secid, "error", err)
		return "", status.Error(codes.Internal, "internal error")
	}
	if len(boards) == 0 {
		return "", status.Errorf(codes.NotFound, "unknown security %s", secid)
	}
	if board == "" {
		if len(boards) > 1 {
			return "", status.Errorf(codes.InvalidArgument, "%s trades on several boards (%s), board is required", secid, strings.Join(boards, ", "))
		}
		return boards[0], nil
	}
	for _, b := range boards {
		if b == board {
			return board, nil
		}
	}
	return "", status.Errorf(codes.NotFound, "unknown security %s on board %s", secid, board)
}

func (s *Server) loadCandles(ctx context.Context, secid, board string, r pricereaderpb.CandleRange) ([]storage.Candle, pricereaderpb.CandleInterval, error) {
	now := s.now()
	loc := s.loc()

	switch r {
	case pricereaderpb.CandleRange_CANDLE_RANGE_DAY:
		// The last day that has hourly candles: today while trading,
		// the previous trading day at night, on weekends and holidays.
		latest, ok, err := s.Store.LatestCandleStart(ctx, secid, board, storage.IntervalHour)
		if err != nil || !ok {
			return nil, pricereaderpb.CandleInterval_CANDLE_INTERVAL_HOUR, err
		}
		l := latest.In(loc)
		dayStart := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
		c, err := s.Store.Candles(ctx, secid, board, storage.IntervalHour, dayStart)
		return c, pricereaderpb.CandleInterval_CANDLE_INTERVAL_HOUR, err

	case pricereaderpb.CandleRange_CANDLE_RANGE_WEEK:
		c, err := s.Store.Candles(ctx, secid, board, storage.IntervalDay, startOfDay(now.AddDate(0, 0, -7), loc))
		return c, pricereaderpb.CandleInterval_CANDLE_INTERVAL_DAY, err

	case pricereaderpb.CandleRange_CANDLE_RANGE_MONTH:
		c, err := s.Store.Candles(ctx, secid, board, storage.IntervalDay, startOfDay(now.AddDate(0, -1, 0), loc))
		return c, pricereaderpb.CandleInterval_CANDLE_INTERVAL_DAY, err

	case pricereaderpb.CandleRange_CANDLE_RANGE_YEAR:
		c, err := s.Store.Candles(ctx, secid, board, storage.IntervalDay, startOfDay(now.AddDate(-1, 0, 0), loc))
		return c, pricereaderpb.CandleInterval_CANDLE_INTERVAL_DAY, err

	default: // ALL
		c, err := s.Store.WeeklyCandles(ctx, secid, board)
		return c, pricereaderpb.CandleInterval_CANDLE_INTERVAL_WEEK, err
	}
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) loc() *time.Location {
	if s.Loc != nil {
		return s.Loc
	}
	return moscowFallback
}
