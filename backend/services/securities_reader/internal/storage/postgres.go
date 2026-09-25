package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type PriceView struct {
	SecID          string
	Board          string
	ShortName      *string
	SecName        *string
	ISIN           *string
	Currency       *string
	Last           *float64
	Open           *float64
	High           *float64
	Low            *float64
	ValueToday     *float64
	VolumeToday    *int64
	TradingStatus  *string
	MoexUpdateTime *string
	CollectedAt    time.Time
}

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

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

const selectLatestPrices = `
	SELECT
		lp.secid, lp.board, sec.short_name, sec.sec_name, sec.isin, sec.currency,
		lp.last_price, lp.open_price, lp.high_price, lp.low_price,
		lp.value_today, lp.volume_today, lp.trading_status, lp.moex_update_time, lp.collected_at
	FROM latest_prices lp
	LEFT JOIN securities sec ON sec.secid = lp.secid AND sec.board = lp.board
`

func (s *Store) AllLatestPrices(ctx context.Context) ([]PriceView, error) {
	query := selectLatestPrices + " ORDER BY lp.secid, lp.board"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query all latest prices: %w", err)
	}
	defer rows.Close()
	return scanPriceRows(rows)
}

func (s *Store) LatestPricesByTickers(ctx context.Context, tickers []string) ([]PriceView, error) {
	if len(tickers) == 0 {
		return nil, nil
	}

	query := selectLatestPrices + " WHERE lp.secid = ANY($1) ORDER BY lp.secid, lp.board"
	rows, err := s.db.QueryContext(ctx, query, pq.Array(tickers))
	if err != nil {
		return nil, fmt.Errorf("query latest prices by tickers: %w", err)
	}
	defer rows.Close()
	return scanPriceRows(rows)
}

func scanPriceRows(rows *sql.Rows) ([]PriceView, error) {
	var out []PriceView
	for rows.Next() {
		var v PriceView
		if err := rows.Scan(
			&v.SecID, &v.Board, &v.ShortName, &v.SecName, &v.ISIN, &v.Currency,
			&v.Last, &v.Open, &v.High, &v.Low,
			&v.ValueToday, &v.VolumeToday, &v.TradingStatus, &v.MoexUpdateTime, &v.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("scan price row: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate price rows: %w", err)
	}
	return out, nil
}
