package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Candle intervals as stored by price_updater in the candles table.
const (
	IntervalHour = "1h"
	IntervalDay  = "1d"
)

type Candle struct {
	Start  time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume *int64
	Value  *float64
}

// BoardsOf lists the boards price_updater knows the security on.
func (s *Store) BoardsOf(ctx context.Context, secid string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT board FROM securities WHERE secid = $1 ORDER BY board`, secid)
	if err != nil {
		return nil, fmt.Errorf("query boards: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, fmt.Errorf("scan board: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// LatestCandleStart returns the start of the newest candle of the interval.
func (s *Store) LatestCandleStart(ctx context.Context, secid, board, interval string) (time.Time, bool, error) {
	var t time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT start_at FROM candles
		WHERE secid = $1 AND board = $2 AND interval = $3
		ORDER BY start_at DESC LIMIT 1
	`, secid, board, interval).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("query latest candle: %w", err)
	}
	return t, true, nil
}

// Candles returns candles of the interval starting at or after from,
// oldest first.
func (s *Store) Candles(ctx context.Context, secid, board, interval string, from time.Time) ([]Candle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT start_at, open, high, low, close, volume, value
		FROM candles
		WHERE secid = $1 AND board = $2 AND interval = $3 AND start_at >= $4
		ORDER BY start_at
	`, secid, board, interval, from)
	if err != nil {
		return nil, fmt.Errorf("query candles: %w", err)
	}
	defer rows.Close()
	return scanCandles(rows)
}

// WeeklyCandles aggregates all stored daily candles into weeks
// (Monday 00:00 Moscow time), oldest first.
func (s *Store) WeeklyCandles(ctx context.Context, secid, board string) ([]Candle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			date_trunc('week', start_at AT TIME ZONE 'Europe/Moscow') AT TIME ZONE 'Europe/Moscow' AS week_start,
			(array_agg(open ORDER BY start_at))[1],
			max(high),
			min(low),
			(array_agg(close ORDER BY start_at DESC))[1],
			sum(volume)::BIGINT,
			sum(value)
		FROM candles
		WHERE secid = $1 AND board = $2 AND interval = '1d'
		GROUP BY week_start
		ORDER BY week_start
	`, secid, board)
	if err != nil {
		return nil, fmt.Errorf("query weekly candles: %w", err)
	}
	defer rows.Close()
	return scanCandles(rows)
}

func scanCandles(rows *sql.Rows) ([]Candle, error) {
	var out []Candle
	for rows.Next() {
		var c Candle
		if err := rows.Scan(&c.Start, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.Value); err != nil {
			return nil, fmt.Errorf("scan candle: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candles: %w", err)
	}
	return out, nil
}
