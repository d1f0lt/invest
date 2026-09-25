package grpcserver

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/securities_reader/internal/storage"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

type Store interface {
	AllLatestPrices(ctx context.Context) ([]storage.PriceView, error)
	LatestPricesByTickers(ctx context.Context, tickers []string) ([]storage.PriceView, error)

	BoardsOf(ctx context.Context, secid string) ([]string, error)
	LatestCandleStart(ctx context.Context, secid, board, interval string) (time.Time, bool, error)
	Candles(ctx context.Context, secid, board, interval string, from time.Time) ([]storage.Candle, error)
	WeeklyCandles(ctx context.Context, secid, board string) ([]storage.Candle, error)

	SearchSecurities(ctx context.Context, query string, limit int) ([]storage.PriceView, error)
}

type Server struct {
	securitiesreaderpb.UnimplementedSecuritiesReaderServiceServer

	Store Store
	Log   *slog.Logger

	
	
	Loc *time.Location
	
	Now func() time.Time
}

func (s *Server) GetPrices(ctx context.Context, req *securitiesreaderpb.GetPricesRequest) (*securitiesreaderpb.GetPricesResponse, error) {
	tickers := normalizeTickers(req.GetTickers())

	var (
		prices []storage.PriceView
		err    error
	)
	if len(tickers) == 0 && len(req.GetTickers()) > 0 {

		return nil, status.Error(codes.InvalidArgument, "tickers field was provided but contained no valid ticker symbols")
	}
	if len(tickers) == 0 {
		prices, err = s.Store.AllLatestPrices(ctx)
	} else {
		prices, err = s.Store.LatestPricesByTickers(ctx, tickers)
	}
	if err != nil {
		s.Log.Error("failed to load prices", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	out := make([]*securitiesreaderpb.PriceView, 0, len(prices))
	for _, p := range prices {
		out = append(out, toPriceViewPB(p))
	}
	return &securitiesreaderpb.GetPricesResponse{Prices: out}, nil
}

func normalizeTickers(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		t := strings.ToUpper(strings.TrimSpace(p))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func toPriceViewPB(v storage.PriceView) *securitiesreaderpb.PriceView {
	return &securitiesreaderpb.PriceView{
		Secid:          v.SecID,
		Board:          v.Board,
		ShortName:      v.ShortName,
		SecName:        v.SecName,
		Isin:           v.ISIN,
		Currency:       v.Currency,
		LastPrice:      v.Last,
		OpenPrice:      v.Open,
		HighPrice:      v.High,
		LowPrice:       v.Low,
		ValueToday:     v.ValueToday,
		VolumeToday:    v.VolumeToday,
		TradingStatus:  v.TradingStatus,
		MoexUpdateTime: v.MoexUpdateTime,
		CollectedAt:    timestamppb.New(v.CollectedAt),
	}
}
