package grpcserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/securities_reader/internal/storage"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

var moscowFallback = time.FixedZone("MSK", 3*60*60)

func (s *Server) GetCandles(ctx context.Context, req *securitiesreaderpb.GetCandlesRequest) (*securitiesreaderpb.GetCandlesResponse, error) {
	secid := strings.ToUpper(strings.TrimSpace(req.GetSecid()))
	if secid == "" {
		return nil, status.Error(codes.InvalidArgument, "secid is required")
	}
	if req.GetRange() == securitiesreaderpb.CandleRange_CANDLE_RANGE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "range is required")
	}
	if _, ok := securitiesreaderpb.CandleRange_name[int32(req.GetRange())]; !ok {
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

	out := make([]*securitiesreaderpb.Candle, 0, len(candles))
	for _, c := range candles {
		out = append(out, &securitiesreaderpb.Candle{
			Start: timestamppb.New(c.Start),
			Open:  c.Open, High: c.High, Low: c.Low, Close: c.Close,
			Volume: c.Volume, Value: c.Value,
		})
	}
	return &securitiesreaderpb.GetCandlesResponse{
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

func (s *Server) loadCandles(ctx context.Context, secid, board string, r securitiesreaderpb.CandleRange) ([]storage.Candle, securitiesreaderpb.CandleInterval, error) {
	now := s.now()
	loc := s.loc()

	switch r {
	case securitiesreaderpb.CandleRange_CANDLE_RANGE_DAY:
		if s.Moex != nil {
			c, err := s.intradayCandles(ctx, secid, board)
			if err == nil && len(c) > 0 {
				return c, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_TEN_MINUTES, nil
			}
			if err != nil {
				s.Log.Warn("moex intraday candles unavailable, falling back to hourly", "secid", secid, "board", board, "error", err)
			}
		}
		latest, ok, err := s.Store.LatestCandleStart(ctx, secid, board, storage.IntervalHour)
		if err != nil || !ok {
			return nil, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_HOUR, err
		}
		l := latest.In(loc)
		dayStart := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
		c, err := s.Store.Candles(ctx, secid, board, storage.IntervalHour, dayStart)
		return c, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_HOUR, err

	case securitiesreaderpb.CandleRange_CANDLE_RANGE_WEEK:
		return s.dailyCandles(ctx, secid, board, startOfDay(now.AddDate(0, 0, -7), loc))

	case securitiesreaderpb.CandleRange_CANDLE_RANGE_MONTH:
		return s.dailyCandles(ctx, secid, board, startOfDay(now.AddDate(0, -1, 0), loc))

	case securitiesreaderpb.CandleRange_CANDLE_RANGE_YEAR:
		return s.dailyCandles(ctx, secid, board, startOfDay(now.AddDate(-1, 0, 0), loc))

	case securitiesreaderpb.CandleRange_CANDLE_RANGE_FIVE_YEARS:
		if s.Moex != nil {
			s.caches()
			c, err := s.moexCandles(ctx, s.longRange, secid, board, 7, startOfDay(now.AddDate(-5, 0, 0), loc))
			if err == nil && len(c) > 0 {
				return c, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_WEEK, nil
			}
			if err != nil {
				s.Log.Warn("moex weekly candles unavailable, falling back to stored history", "secid", secid, "board", board, "error", err)
			}
		}
		c, err := s.Store.WeeklyCandles(ctx, secid, board)
		return c, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_WEEK, err

	default: 
		c, err := s.Store.WeeklyCandles(ctx, secid, board)
		return c, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_WEEK, err
	}
}

func (s *Server) intradayCandles(ctx context.Context, secid, board string) ([]storage.Candle, error) {
	s.caches()
	loc := s.loc()
	day := s.now()
	latest, ok, err := s.Store.LatestCandleStart(ctx, secid, board, storage.IntervalDay)
	if err != nil {
		return nil, err
	}
	if ok {
		day = latest
	}
	date := day.In(loc).Format("2006-01-02")
	key := secid + "|" + board + "|" + date
	if c, ok := s.intraday.get(key, s.now()); ok {
		return c, nil
	}

	raw, err := s.Moex.IntradayCandles(ctx, secid, board, day, loc)
	if err != nil {
		return nil, err
	}
	out := make([]storage.Candle, 0, len(raw))
	for _, c := range raw {
		out = append(out, storage.Candle{Start: c.Begin, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close})
	}
	s.intraday.put(key, out, s.now())
	return out, nil
}

func (s *Server) dailyCandles(ctx context.Context, secid, board string, from time.Time) ([]storage.Candle, securitiesreaderpb.CandleInterval, error) {
	if s.Moex != nil {
		s.caches()
		c, err := s.moexCandles(ctx, s.daily, secid, board, 24, from)
		if err == nil && len(c) > 0 {
			return c, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_DAY, nil
		}
		if err != nil {
			s.Log.Warn("moex daily candles unavailable, falling back to stored history", "secid", secid, "board", board, "error", err)
		}
	}
	c, err := s.Store.Candles(ctx, secid, board, storage.IntervalDay, from)
	return c, securitiesreaderpb.CandleInterval_CANDLE_INTERVAL_DAY, err
}

func (s *Server) moexCandles(ctx context.Context, cache *ttlCache[[]storage.Candle], secid, board string, interval int, from time.Time) ([]storage.Candle, error) {
	s.caches()
	loc := s.loc()
	now := s.now()
	key := fmt.Sprintf("%s|%s|%d|%s", secid, board, interval, from.In(loc).Format("2006-01-02"))
	if c, ok := cache.get(key, now); ok {
		return c, nil
	}
	raw, err := s.Moex.Candles(ctx, secid, board, interval, from, now, loc)
	if err != nil {
		return nil, err
	}
	out := make([]storage.Candle, 0, len(raw))
	for _, c := range raw {
		out = append(out, storage.Candle{Start: c.Begin, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close})
	}
	cache.put(key, out, now)
	return out, nil
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
