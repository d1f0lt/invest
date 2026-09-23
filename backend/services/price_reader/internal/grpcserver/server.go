package grpcserver

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/price_reader/internal/storage"
	pricereaderpb "invest/backend/services/price_reader/proto"
)

type Store interface {
	AllLatestPrices(ctx context.Context) ([]storage.PriceView, error)
	LatestPricesByTickers(ctx context.Context, tickers []string) ([]storage.PriceView, error)
}

type Server struct {
	pricereaderpb.UnimplementedPriceReaderServiceServer

	Store Store
	Log   *slog.Logger
}

func (s *Server) GetPrices(ctx context.Context, req *pricereaderpb.GetPricesRequest) (*pricereaderpb.GetPricesResponse, error) {
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

	out := make([]*pricereaderpb.PriceView, 0, len(prices))
	for _, p := range prices {
		out = append(out, toPriceViewPB(p))
	}
	return &pricereaderpb.GetPricesResponse{Prices: out}, nil
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

func toPriceViewPB(v storage.PriceView) *pricereaderpb.PriceView {
	return &pricereaderpb.PriceView{
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
