package updater

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"invest/backend/services/price_updater/internal/moexclient"
	"invest/backend/services/price_updater/internal/storage"
)

type intradayFake struct {
	saved  []storage.Candle
	mode   storage.CandleMode
	calls  int
	pruned bool
}

func (f *intradayFake) IntradayTargets(context.Context, []string) ([]storage.IntradayTarget, error) {
	return []storage.IntradayTarget{{SecID: "BAD", Board: "TQBR", Day: time.Now()}, {SecID: "SBER", Board: "TQBR", Day: time.Now()}}, nil
}
func (f *intradayFake) FetchIntradayCandles(_ context.Context, secid, _ string, day time.Time, _ *time.Location) ([]moexclient.DailyCandle, error) {
	f.calls++
	if secid == "BAD" {
		return nil, errors.New("unavailable")
	}
	volume := int64(42)
	return []moexclient.DailyCandle{{TradeDate: day, Open: 1, High: 3, Low: 1, Close: 2, Volume: &volume}}, nil
}
func (f *intradayFake) UpsertCandles(_ context.Context, candles []storage.Candle, mode storage.CandleMode) (int64, error) {
	f.saved = append(f.saved, candles...)
	f.mode = mode
	return int64(len(candles)), nil
}
func (f *intradayFake) PruneIntradayCandles(context.Context) (int64, error) {
	f.pruned = true
	return 0, nil
}

func TestIntradayJobContinuesAfterFailureAndReplacesCandles(t *testing.T) {
	f := &intradayFake{}
	job := IntradayJob{Client: f, Store: f, Loc: time.UTC, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	job.RunOnce(context.Background())
	if f.calls != 2 || len(f.saved) != 1 || !f.pruned || f.mode != storage.CandleReplace {
		t.Fatalf("result=%+v", f)
	}
	c := f.saved[0]
	if c.SecID != "SBER" || c.Board != "TQBR" || c.Interval != storage.IntervalTenMinutes || c.Close != 2 || c.Volume == nil || *c.Volume != 42 {
		t.Fatalf("candle=%+v", c)
	}
}
