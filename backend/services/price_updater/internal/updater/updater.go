package updater

import (
	"context"
	"log/slog"
	"time"

	"invest/backend/services/price_updater/internal/moexclient"
	"invest/backend/services/price_updater/internal/storage"
)

type MoexClient interface {
	FetchBoard(ctx context.Context, board string) (moexclient.BoardSnapshot, error)
	FetchBoardHistory(ctx context.Context, board string, date time.Time) ([]moexclient.DailyCandle, error)
}

type Storage interface {
	UpsertSecurities(ctx context.Context, securities []moexclient.Security) error
	UpsertLatestPrices(ctx context.Context, rows []storage.PriceRow) (int64, error)
	UpsertCandles(ctx context.Context, candles []storage.Candle, mode storage.CandleMode) (int64, error)
	HistorySyncedThrough(ctx context.Context, board string) (time.Time, bool, error)
	SetHistorySyncedThrough(ctx context.Context, board string, date time.Time) error
	PruneHourlyCandles(ctx context.Context) (int64, error)
}

type Updater struct {
	client MoexClient
	store  Storage
	boards []string
	loc    *time.Location
	log    *slog.Logger
	now    func() time.Time
}

func New(client MoexClient, store Storage, boards []string, loc *time.Location, log *slog.Logger) *Updater {
	return &Updater{client: client, store: store, boards: boards, loc: loc, log: log, now: time.Now}
}

func (u *Updater) Run(ctx context.Context, interval time.Duration) {
	u.runCycle(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			u.log.Info("updater stopping")
			return
		case <-ticker.C:
			u.runCycle(ctx)
		}
	}
}

func (u *Updater) runCycle(ctx context.Context) {
	start := u.now()
	collectedAt := start.UTC()

	var stats cycleStats
	var boardsOK int

	for _, board := range u.boards {
		if err := u.syncBoard(ctx, board, collectedAt, &stats); err != nil {
			u.log.Error("board sync failed", "board", board, "error", err)
			continue
		}
		boardsOK++
	}

	u.log.Info("update cycle complete",
		"boards_ok", boardsOK,
		"boards_total", len(u.boards),
		"prices_upserted", stats.prices,
		"hourly_candles_upserted", stats.hourly,
		"daily_candles_upserted", stats.daily,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

type cycleStats struct {
	prices, hourly, daily int64
}

func (u *Updater) syncBoard(ctx context.Context, board string, collectedAt time.Time, stats *cycleStats) error {
	snapshot, err := u.client.FetchBoard(ctx, board)
	if err != nil {
		return err
	}

	if err := u.store.UpsertSecurities(ctx, snapshot.Securities); err != nil {
		return err
	}

	prices, hourly, daily := buildRows(board, snapshot.Quotes, collectedAt, u.loc)

	n, err := u.store.UpsertLatestPrices(ctx, prices)
	if err != nil {
		return err
	}
	stats.prices += n

	n, err = u.store.UpsertCandles(ctx, hourly, storage.CandleExtend)
	if err != nil {
		return err
	}
	stats.hourly += n

	n, err = u.store.UpsertCandles(ctx, daily, storage.CandleReplace)
	if err != nil {
		return err
	}
	stats.daily += n
	return nil
}

const tradingStatusTrading = "T"

func buildRows(board string, quotes map[string]moexclient.MarketQuote, collectedAt time.Time, loc *time.Location) (prices []storage.PriceRow, hourly, daily []storage.Candle) {
	local := collectedAt.In(loc)
	hourStart := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, loc)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)

	prices = make([]storage.PriceRow, 0, len(quotes))
	for secID, q := range quotes {
		prices = append(prices, storage.PriceRow{
			SecID:          secID,
			Board:          board,
			Last:           q.Last,
			Open:           q.Open,
			High:           q.High,
			Low:            q.Low,
			ValueToday:     q.ValueToday,
			VolumeToday:    q.VolumeToday,
			TradingStatus:  q.TradingStatus,
			MoexUpdateTime: q.UpdateTime,
			CollectedAt:    collectedAt,
			PrevClose:      q.PrevClose,
		})

		if q.Last == nil || q.TradingStatus == nil || *q.TradingStatus != tradingStatusTrading {
			continue
		}
		last := *q.Last

		hourly = append(hourly, storage.Candle{
			SecID: secID, Board: board, Interval: storage.IntervalHour, StartAt: hourStart,
			Open: last, High: last, Low: last, Close: last,
		})

		if q.Open == nil {
			continue
		}
		d := storage.Candle{
			SecID: secID, Board: board, Interval: storage.IntervalDay, StartAt: dayStart,
			Open: *q.Open, High: maxOf(*q.Open, last, q.High), Low: minOf(*q.Open, last, q.Low), Close: last,
			Volume: q.VolumeToday, Value: q.ValueToday,
		}
		daily = append(daily, d)
	}
	return prices, hourly, daily
}

func maxOf(a, b float64, c *float64) float64 {
	m := max(a, b)
	if c != nil {
		m = max(m, *c)
	}
	return m
}

func minOf(a, b float64, c *float64) float64 {
	m := min(a, b)
	if c != nil {
		m = min(m, *c)
	}
	return m
}
