package updater

import (
	"context"
	"log/slog"
	"time"

	"invest/backend/services/price_updater/internal/storage"
)

type HistoryConfig struct {
	
	RunAtHour, RunAtMinute int
	
	
	BackfillDays int
	
	
	RequestPause time.Duration
}







type HistoryJob struct {
	client MoexClient
	store  Storage
	boards []string
	loc    *time.Location
	cfg    HistoryConfig
	log    *slog.Logger
	now    func() time.Time
}

func NewHistoryJob(client MoexClient, store Storage, boards []string, loc *time.Location, cfg HistoryConfig, log *slog.Logger) *HistoryJob {
	return &HistoryJob{client: client, store: store, boards: boards, loc: loc, cfg: cfg, log: log, now: time.Now}
}

func (j *HistoryJob) Run(ctx context.Context) {
	for {
		j.RunOnce(ctx)

		next := nextDailyRun(j.now(), j.loc, j.cfg.RunAtHour, j.cfg.RunAtMinute)
		j.log.Info("history job scheduled", "next_run", next.Format(time.RFC3339))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			j.log.Info("history job stopping")
			return
		case <-timer.C:
		}
	}
}

func (j *HistoryJob) RunOnce(ctx context.Context) {
	start := j.now()
	today := dateOf(start, j.loc)

	for _, board := range j.boards {
		if ctx.Err() != nil {
			return
		}
		if err := j.syncBoard(ctx, board, today); err != nil {
			j.log.Error("history sync failed", "board", board, "error", err)
		}
	}

	n, err := j.store.PruneHourlyCandles(ctx)
	if err != nil {
		j.log.Error("hourly candle cleanup failed", "error", err)
	} else {
		j.log.Info("hourly candles cleaned up", "deleted", n)
	}

	j.log.Info("history job complete", "duration_ms", time.Since(start).Milliseconds())
}

func (j *HistoryJob) syncBoard(ctx context.Context, board string, today time.Time) error {
	last, ok, err := j.store.HistorySyncedThrough(ctx, board)
	if err != nil {
		return err
	}
	from := today.AddDate(0, 0, -j.cfg.BackfillDays)
	if ok {
		from = last.AddDate(0, 0, 1)
	}
	yesterday := today.AddDate(0, 0, -1)

	loaded := 0
	for d := from; !d.After(yesterday); d = d.AddDate(0, 0, 1) {
		if loaded > 0 && j.cfg.RequestPause > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(j.cfg.RequestPause):
			}
		}

		daily, err := j.client.FetchBoardHistory(ctx, board, d)
		if err != nil {
			return err
		}

		
		
		
		if len(daily) == 0 && d.Equal(yesterday) {
			j.log.Warn("no history yet for yesterday, will retry next run", "board", board, "date", d.Format("2006-01-02"))
			break
		}

		candles := make([]storage.Candle, 0, len(daily))
		for _, c := range daily {
			candles = append(candles, storage.Candle{
				SecID: c.SecID, Board: c.BoardID, Interval: storage.IntervalDay,
				StartAt: time.Date(c.TradeDate.Year(), c.TradeDate.Month(), c.TradeDate.Day(), 0, 0, 0, 0, j.loc),
				Open:    c.Open, High: c.High, Low: c.Low, Close: c.Close,
				Volume: c.Volume, Value: c.Value,
			})
		}
		if _, err := j.store.UpsertCandles(ctx, candles, storage.CandleReplace); err != nil {
			return err
		}
		if err := j.store.SetHistorySyncedThrough(ctx, board, d); err != nil {
			return err
		}
		loaded++
		j.log.Debug("history loaded", "board", board, "date", d.Format("2006-01-02"), "candles", len(candles))
	}

	if loaded > 0 {
		j.log.Info("history synced", "board", board, "dates_loaded", loaded, "through", yesterday.Format("2006-01-02"))
	}
	return nil
}



func dateOf(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}


func nextDailyRun(now time.Time, loc *time.Location, hour, minute int) time.Time {
	l := now.In(loc)
	next := time.Date(l.Year(), l.Month(), l.Day(), hour, minute, 0, 0, loc)
	if !next.After(l) {
		next = time.Date(l.Year(), l.Month(), l.Day()+1, hour, minute, 0, 0, loc)
	}
	return next
}
