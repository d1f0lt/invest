package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type DailyClose struct {
	Day   time.Time
	Close float64
}

func instrumentArrays(instruments [][2]string) (pq.StringArray, pq.StringArray) {
	secids := make(pq.StringArray, len(instruments))
	boards := make(pq.StringArray, len(instruments))
	for i, inst := range instruments {
		secids[i], boards[i] = inst[0], inst[1]
	}
	return secids, boards
}

func (s *Store) PrevCloses(ctx context.Context, instruments [][2]string) (map[string]float64, error) {
	out := make(map[string]float64, len(instruments))
	if len(instruments) == 0 {
		return out, nil
	}
	secids, boards := instrumentArrays(instruments)
	const stmt = `
		SELECT lp.secid, lp.board,
		       CASE WHEN s.price_in_percent AND s.face_value IS NOT NULL
		            THEN lp.prev_close * s.face_value / 100
		            ELSE lp.prev_close
		       END
		FROM latest_prices lp
		JOIN securities s ON s.secid = lp.secid AND s.board = lp.board
		WHERE (lp.secid, lp.board) IN (
			SELECT * FROM unnest($1::text[], $2::text[])
		)
		AND lp.prev_close IS NOT NULL
	`
	rows, err := s.db.QueryContext(ctx, stmt, secids, boards)
	if err != nil {
		return nil, fmt.Errorf("select prev closes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var secid, board string
		var price float64
		if err := rows.Scan(&secid, &board, &price); err != nil {
			return nil, fmt.Errorf("scan prev close: %w", err)
		}
		out[secid+"/"+board] = price
	}
	return out, rows.Err()
}

func (s *Store) DailyCloses(ctx context.Context, instruments [][2]string, from time.Time) (map[string][]DailyClose, error) {
	out := make(map[string][]DailyClose, len(instruments))
	if len(instruments) == 0 {
		return out, nil
	}
	secids, boards := instrumentArrays(instruments)
	const stmt = `
		SELECT c.secid, c.board, c.start_at,
		       CASE WHEN s.price_in_percent AND s.face_value IS NOT NULL
		            THEN c.close * s.face_value / 100
		            ELSE c.close
		       END
		FROM candles c
		JOIN securities s ON s.secid = c.secid AND s.board = c.board
		WHERE c.interval = '1d'
		AND c.start_at >= $3
		AND (c.secid, c.board) IN (
			SELECT * FROM unnest($1::text[], $2::text[])
		)
		ORDER BY c.secid, c.board, c.start_at
	`
	rows, err := s.db.QueryContext(ctx, stmt, secids, boards, from)
	if err != nil {
		return nil, fmt.Errorf("select daily closes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var secid, board string
		var c DailyClose
		if err := rows.Scan(&secid, &board, &c.Day, &c.Close); err != nil {
			return nil, fmt.Errorf("scan daily close: %w", err)
		}
		key := secid + "/" + board
		out[key] = append(out[key], c)
	}
	return out, rows.Err()
}
