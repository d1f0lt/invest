package updater

import (
	"context"
	"log/slog"
	"time"

	"invest/backend/services/price_updater/internal/moexclient"
	"invest/backend/services/price_updater/internal/storage"
)

type IntradayClient interface {
	FetchIntradayCandles(context.Context, string, string, time.Time, *time.Location) ([]moexclient.DailyCandle, error)
}

type IntradayStore interface {
	IntradayTargets(context.Context, []string) ([]storage.IntradayTarget, error)
	UpsertCandles(context.Context, []storage.Candle, storage.CandleMode) (int64, error)
	PruneIntradayCandles(context.Context) (int64, error)
}

type IntradayJob struct {
	Client       IntradayClient
	Store        IntradayStore
	Boards       []string
	Loc          *time.Location
	Log          *slog.Logger
	RequestPause time.Duration
}

func (j *IntradayJob) Run(ctx context.Context, interval time.Duration) {
	for {
		j.RunOnce(ctx)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (j *IntradayJob) RunOnce(ctx context.Context) {
	targets, err := j.Store.IntradayTargets(ctx, j.Boards)
	if err != nil {
		j.Log.Error("intraday targets failed", "error", err)
		return
	}
	for i, target := range targets {
		if ctx.Err() != nil {
			return
		}
		if i > 0 && j.RequestPause > 0 {
			timer := time.NewTimer(j.RequestPause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		raw, err := j.Client.FetchIntradayCandles(ctx, target.SecID, target.Board, target.Day, j.Loc)
		if err != nil {
			j.Log.Warn("intraday fetch failed", "secid", target.SecID, "board", target.Board, "error", err)
			continue
		}
		candles := make([]storage.Candle, 0, len(raw))
		for _, c := range raw {
			candles = append(candles, storage.Candle{SecID: target.SecID, Board: target.Board, Interval: storage.IntervalTenMinutes, StartAt: c.TradeDate, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Value: c.Value})
		}
		if _, err := j.Store.UpsertCandles(ctx, candles, storage.CandleReplace); err != nil {
			j.Log.Error("intraday save failed", "secid", target.SecID, "board", target.Board, "error", err)
		}
	}
	if _, err := j.Store.PruneIntradayCandles(ctx); err != nil {
		j.Log.Error("intraday cleanup failed", "error", err)
	}
}
