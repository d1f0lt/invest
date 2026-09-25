package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"invest/backend/services/price_updater/internal/moexclient"
)

type Store struct {
	db *sql.DB
}

func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() {
	s.db.Close()
}

func (s *Store) UpsertSecurities(ctx context.Context, securities []moexclient.Security) error {
	if len(securities) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const stmt = `
		INSERT INTO securities (secid, board, short_name, sec_name, isin, currency, lot_size, decimals, face_value, price_in_percent, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		ON CONFLICT (secid, board) DO UPDATE SET
			short_name = EXCLUDED.short_name,
			sec_name   = EXCLUDED.sec_name,
			isin       = EXCLUDED.isin,
			currency   = EXCLUDED.currency,
			lot_size   = EXCLUDED.lot_size,
			decimals   = EXCLUDED.decimals,
			face_value = EXCLUDED.face_value,
			price_in_percent = EXCLUDED.price_in_percent,
			updated_at = now()
	`

	prepared, err := tx.PrepareContext(ctx, stmt)
	if err != nil {
		return fmt.Errorf("prepare upsert securities: %w", err)
	}
	defer prepared.Close()

	for _, sec := range securities {
		if _, err := prepared.ExecContext(ctx,
			sec.SecID, sec.BoardID, sec.ShortName, sec.SecName, sec.ISIN, sec.Currency, sec.LotSize, sec.Decimals,
			sec.FaceValue, sec.PriceInPercent,
		); err != nil {
			return fmt.Errorf("upsert security %s/%s: %w", sec.BoardID, sec.SecID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert securities: %w", err)
	}
	return nil
}

type PriceRow struct {
	SecID          string
	Board          string
	Last           *float64
	Open           *float64
	High           *float64
	Low            *float64
	ValueToday     *float64
	VolumeToday    *int64
	TradingStatus  *string
	MoexUpdateTime *string
	CollectedAt    time.Time
	PrevClose      *float64
}




func (s *Store) UpsertLatestPrices(ctx context.Context, rows []PriceRow) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE tmp_latest_prices (LIKE latest_prices INCLUDING DEFAULTS) ON COMMIT DROP`); err != nil {
		return 0, fmt.Errorf("create temp table: %w", err)
	}

	columns := []string{
		"secid", "board", "last_price", "open_price", "high_price", "low_price",
		"value_today", "volume_today", "trading_status", "moex_update_time", "collected_at",
		"prev_close",
	}
	values := make([][]any, 0, len(rows))
	for _, r := range rows {
		values = append(values, []any{
			r.SecID, r.Board, r.Last, r.Open, r.High, r.Low,
			r.ValueToday, r.VolumeToday, r.TradingStatus, r.MoexUpdateTime, r.CollectedAt,
			r.PrevClose,
		})
	}
	if err := copyRows(ctx, tx, "tmp_latest_prices", columns, values); err != nil {
		return 0, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO latest_prices AS lp (
			secid, board, last_price, open_price, high_price, low_price,
			value_today, volume_today, trading_status, moex_update_time, collected_at,
			prev_close
		)
		SELECT DISTINCT ON (secid, board)
			secid, board, last_price, open_price, high_price, low_price,
			value_today, volume_today, trading_status, moex_update_time, collected_at,
			prev_close
		FROM tmp_latest_prices
		ORDER BY secid, board
		ON CONFLICT (secid, board) DO UPDATE SET
			last_price       = COALESCE(EXCLUDED.last_price, lp.last_price),
			open_price       = EXCLUDED.open_price,
			high_price       = EXCLUDED.high_price,
			low_price        = EXCLUDED.low_price,
			value_today      = EXCLUDED.value_today,
			volume_today     = EXCLUDED.volume_today,
			trading_status   = EXCLUDED.trading_status,
			moex_update_time = EXCLUDED.moex_update_time,
			collected_at     = EXCLUDED.collected_at,
			prev_close       = COALESCE(EXCLUDED.prev_close, lp.prev_close)
	`)
	if err != nil {
		return 0, fmt.Errorf("upsert latest prices: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit latest prices: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE latest_prices ADD COLUMN IF NOT EXISTS prev_close NUMERIC`); err != nil {
		return fmt.Errorf("ensure latest_prices.prev_close: %w", err)
	}
	return nil
}

const (
	IntervalHour = "1h"
	IntervalDay  = "1d"
)

type Candle struct {
	SecID    string
	Board    string
	Interval string
	StartAt  time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   *int64
	Value    *float64
}



type CandleMode int

const (
	
	
	
	
	CandleExtend CandleMode = iota
	
	
	
	CandleReplace
)

func (s *Store) UpsertCandles(ctx context.Context, candles []Candle, mode CandleMode) (int64, error) {
	if len(candles) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE tmp_candles (LIKE candles INCLUDING DEFAULTS) ON COMMIT DROP`); err != nil {
		return 0, fmt.Errorf("create temp table: %w", err)
	}

	columns := []string{"secid", "board", "interval", "start_at", "open", "high", "low", "close", "volume", "value"}
	values := make([][]any, 0, len(candles))
	for _, c := range candles {
		values = append(values, []any{
			c.SecID, c.Board, c.Interval, c.StartAt, c.Open, c.High, c.Low, c.Close, c.Volume, c.Value,
		})
	}
	if err := copyRows(ctx, tx, "tmp_candles", columns, values); err != nil {
		return 0, err
	}

	onConflict := `
		open       = EXCLUDED.open,
		high       = EXCLUDED.high,
		low        = EXCLUDED.low,
		close      = EXCLUDED.close,
		volume     = EXCLUDED.volume,
		value      = EXCLUDED.value,
		updated_at = now()`
	if mode == CandleExtend {
		onConflict = `
		high       = GREATEST(c.high, EXCLUDED.high),
		low        = LEAST(c.low, EXCLUDED.low),
		close      = EXCLUDED.close,
		updated_at = now()`
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO candles AS c (secid, board, interval, start_at, open, high, low, close, volume, value)
		SELECT DISTINCT ON (secid, board, interval, start_at)
			secid, board, interval, start_at, open, high, low, close, volume, value
		FROM tmp_candles
		ORDER BY secid, board, interval, start_at
		ON CONFLICT (secid, board, interval, start_at) DO UPDATE SET`+onConflict)
	if err != nil {
		return 0, fmt.Errorf("upsert candles: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit candles: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}



func (s *Store) HistorySyncedThrough(ctx context.Context, board string) (time.Time, bool, error) {
	var d time.Time
	err := s.db.QueryRowContext(ctx, `SELECT synced_through FROM candle_history_sync WHERE board = $1`, board).Scan(&d)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("select history sync state: %w", err)
	}
	return d, true, nil
}

func (s *Store) SetHistorySyncedThrough(ctx context.Context, board string, date time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO candle_history_sync (board, synced_through, updated_at)
		VALUES ($1, $2::date, now())
		ON CONFLICT (board) DO UPDATE SET synced_through = EXCLUDED.synced_through, updated_at = now()
	`, board, date.Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("update history sync state: %w", err)
	}
	return nil
}





func (s *Store) PruneHourlyCandles(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM candles c
		USING (
			SELECT secid, board,
			       date_trunc('day', max(start_at) AT TIME ZONE 'Europe/Moscow') AT TIME ZONE 'Europe/Moscow' AS last_day
			FROM candles
			WHERE interval = '1h'
			GROUP BY secid, board
		) l
		WHERE c.interval = '1h'
		  AND c.secid = l.secid AND c.board = l.board
		  AND c.start_at < l.last_day
	`)
	if err != nil {
		return 0, fmt.Errorf("prune hourly candles: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func copyRows(ctx context.Context, tx *sql.Tx, table string, columns []string, rows [][]any) error {
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(table, columns...))
	if err != nil {
		return fmt.Errorf("prepare copy into %s: %w", table, err)
	}
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r...); err != nil {
			stmt.Close()
			return fmt.Errorf("copy row into %s: %w", table, err)
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		stmt.Close()
		return fmt.Errorf("flush copy into %s: %w", table, err)
	}
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close copy into %s: %w", table, err)
	}
	return nil
}
