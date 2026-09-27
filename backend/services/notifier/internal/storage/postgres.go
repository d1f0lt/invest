package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrAmbiguousBoard = errors.New("security trades on several boards")
)

const postgresInvalidTextRepresentation = "22P02"

func isInvalidText(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == postgresInvalidTextRepresentation
}

const (
	DirectionAbove = "above"
	DirectionBelow = "below"
)

type Instrument struct {
	SecID          string
	Board          string
	ShortName      string
	Currency       string
	Decimals       *int
	PriceInPercent bool
	LastPrice      *float64
}

type Alert struct {
	ID          string
	UserID      string
	SecID       string
	Board       string
	Direction   string
	BasePrice   float64
	TargetPrice float64
	CreatedAt   time.Time
	UpdatedAt   time.Time

	TriggeredAt    *time.Time
	TriggeredPrice *float64
	InputPercent   *float64

	ShortName      string
	Currency       string
	PriceInPercent bool
	CurrentPrice   *float64
}

type TriggeredAlert struct {
	Alert
	ChatID    int64
	LastPrice float64
}

type TelegramLink struct {
	UserID   string
	ChatID   int64
	Username string
	LinkedAt time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
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

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Instrument(ctx context.Context, secid, board string) (Instrument, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.secid, s.board, COALESCE(s.short_name, s.secid), COALESCE(s.currency, ''),
		       s.decimals, s.price_in_percent, lp.last_price
		FROM securities s
		LEFT JOIN latest_prices lp ON lp.secid = s.secid AND lp.board = s.board
		WHERE s.secid = $1 AND ($2 = '' OR s.board = $2)
		ORDER BY s.board
	`, secid, board)
	if err != nil {
		return Instrument{}, fmt.Errorf("select instrument: %w", err)
	}
	defer rows.Close()

	var found []Instrument
	for rows.Next() {
		var in Instrument
		var decimals sql.NullInt64
		var last sql.NullFloat64
		if err := rows.Scan(&in.SecID, &in.Board, &in.ShortName, &in.Currency, &decimals, &in.PriceInPercent, &last); err != nil {
			return Instrument{}, fmt.Errorf("scan instrument: %w", err)
		}
		if decimals.Valid {
			d := int(decimals.Int64)
			in.Decimals = &d
		}
		if last.Valid {
			in.LastPrice = &last.Float64
		}
		found = append(found, in)
	}
	if err := rows.Err(); err != nil {
		return Instrument{}, fmt.Errorf("iterate instruments: %w", err)
	}
	switch len(found) {
	case 0:
		return Instrument{}, ErrNotFound
	case 1:
		return found[0], nil
	default:
		return Instrument{}, ErrAmbiguousBoard
	}
}

func (s *Store) CountAlerts(ctx context.Context, userID string) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM alerts WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count alerts: %w", err)
	}
	return n, nil
}

func (s *Store) CreateAlert(ctx context.Context, a Alert) (Alert, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO alerts (user_id, secid, board, direction, base_price, target_price, input_percent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`, a.UserID, a.SecID, a.Board, a.Direction, a.BasePrice, a.TargetPrice, a.InputPercent).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Alert{}, fmt.Errorf("insert alert: %w", err)
	}
	return a, nil
}

const alertColumns = `
	a.id, a.user_id, a.secid, a.board, a.direction, a.base_price, a.target_price,
	a.created_at, a.updated_at, a.triggered_at, a.triggered_price, a.input_percent,
	COALESCE(s.short_name, a.secid), COALESCE(s.currency, ''), COALESCE(s.price_in_percent, false)`

type scanner interface{ Scan(dest ...any) error }

func scanAlert(row scanner, extra ...any) (Alert, error) {
	var a Alert
	var triggeredAt sql.NullTime
	var triggeredPrice, inputPercent sql.NullFloat64
	dest := []any{
		&a.ID, &a.UserID, &a.SecID, &a.Board, &a.Direction, &a.BasePrice, &a.TargetPrice,
		&a.CreatedAt, &a.UpdatedAt, &triggeredAt, &triggeredPrice, &inputPercent,
		&a.ShortName, &a.Currency, &a.PriceInPercent,
	}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return Alert{}, err
	}
	if triggeredAt.Valid {
		a.TriggeredAt = &triggeredAt.Time
	}
	if triggeredPrice.Valid {
		a.TriggeredPrice = &triggeredPrice.Float64
	}
	if inputPercent.Valid {
		a.InputPercent = &inputPercent.Float64
	}
	return a, nil
}

func (s *Store) ListAlerts(ctx context.Context, userID string) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+alertColumns+`, lp.last_price
		FROM alerts a
		LEFT JOIN securities s ON s.secid = a.secid AND s.board = a.board
		LEFT JOIN latest_prices lp ON lp.secid = a.secid AND lp.board = a.board
		WHERE a.user_id = $1
		ORDER BY a.created_at, a.id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("select alerts: %w", err)
	}
	defer rows.Close()

	out := []Alert{}
	for rows.Next() {
		var last sql.NullFloat64
		a, err := scanAlert(rows, &last)
		if err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		if last.Valid {
			a.CurrentPrice = &last.Float64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAlert(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alerts WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		if isInvalidText(err) {
			return ErrNotFound
		}
		return fmt.Errorf("delete alert: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetAlert(ctx context.Context, userID, id string) (Alert, error) {
	var last sql.NullFloat64
	a, err := scanAlert(s.db.QueryRowContext(ctx, `
		SELECT `+alertColumns+`, lp.last_price
		FROM alerts a
		LEFT JOIN securities s ON s.secid = a.secid AND s.board = a.board
		LEFT JOIN latest_prices lp ON lp.secid = a.secid AND lp.board = a.board
		WHERE a.id = $1 AND a.user_id = $2
	`, id, userID), &last)
	if errors.Is(err, sql.ErrNoRows) || isInvalidText(err) {
		return Alert{}, ErrNotFound
	}
	if err != nil {
		return Alert{}, fmt.Errorf("select alert: %w", err)
	}
	if last.Valid {
		a.CurrentPrice = &last.Float64
	}
	return a, nil
}

func (s *Store) UpdateAlert(ctx context.Context, a Alert) (Alert, error) {
	var triggeredAt sql.NullTime
	var triggeredPrice sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
		UPDATE alerts
		SET secid = $3, board = $4, direction = $5, base_price = $6, target_price = $7,
		    input_percent = $8, updated_at = now(), triggered_at = NULL, triggered_price = NULL
		WHERE id = $1 AND user_id = $2
		RETURNING created_at, updated_at, triggered_at, triggered_price
	`, a.ID, a.UserID, a.SecID, a.Board, a.Direction, a.BasePrice, a.TargetPrice, a.InputPercent).Scan(&a.CreatedAt, &a.UpdatedAt, &triggeredAt, &triggeredPrice)
	if errors.Is(err, sql.ErrNoRows) || isInvalidText(err) {
		return Alert{}, ErrNotFound
	}
	if err != nil {
		return Alert{}, fmt.Errorf("update alert: %w", err)
	}
	a.TriggeredAt, a.TriggeredPrice = nil, nil
	return a, nil
}

func (s *Store) TriggeredAlerts(ctx context.Context, staleAfter time.Duration) ([]TriggeredAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+alertColumns+`, tl.chat_id, lp.last_price
		FROM alerts a
		JOIN telegram_links tl ON tl.user_id = a.user_id
		JOIN latest_prices lp ON lp.secid = a.secid AND lp.board = a.board
		LEFT JOIN securities s ON s.secid = a.secid AND s.board = a.board
		WHERE a.triggered_at IS NULL
		  AND lp.trading_status = 'T'
		  AND lp.last_price IS NOT NULL
		  AND lp.collected_at > now() - make_interval(secs => $1)
		  AND (
		        (a.direction = 'above' AND lp.last_price >= a.target_price)
		     OR (a.direction = 'below' AND lp.last_price <= a.target_price)
		  )
		ORDER BY a.created_at, a.id
	`, staleAfter.Seconds())
	if err != nil {
		return nil, fmt.Errorf("select triggered alerts: %w", err)
	}
	defer rows.Close()

	var out []TriggeredAlert
	for rows.Next() {
		var t TriggeredAlert
		a, err := scanAlert(rows, &t.ChatID, &t.LastPrice)
		if err != nil {
			return nil, fmt.Errorf("scan triggered alert: %w", err)
		}
		t.Alert = a
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) MarkTriggered(ctx context.Context, id string, updatedAt time.Time, price float64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE alerts SET triggered_at = now(), triggered_price = $3
		WHERE id = $1 AND updated_at = $2 AND triggered_at IS NULL
	`, id, updatedAt, price)
	if err != nil {
		return fmt.Errorf("mark alert triggered: %w", err)
	}
	return nil
}

func (s *Store) CreateLinkToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM telegram_link_tokens WHERE user_id = $1 OR expires_at <= now()`, userID); err != nil {
		return fmt.Errorf("delete old link tokens: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO telegram_link_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)
	`, tokenHash, userID, expiresAt); err != nil {
		return fmt.Errorf("insert link token: %w", err)
	}
	return tx.Commit()
}

func (s *Store) ConsumeLinkToken(ctx context.Context, tokenHash string, chatID int64, username string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRowContext(ctx, `
		DELETE FROM telegram_link_tokens WHERE token_hash = $1 AND expires_at > now()
		RETURNING user_id
	`, tokenHash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("consume link token: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM telegram_links WHERE chat_id = $1 AND user_id <> $2`, chatID, userID); err != nil {
		return "", fmt.Errorf("unlink chat from previous user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO telegram_links (user_id, chat_id, username, linked_at)
		VALUES ($1, $2, NULLIF($3, ''), now())
		ON CONFLICT (user_id) DO UPDATE SET
			chat_id = EXCLUDED.chat_id, username = EXCLUDED.username, linked_at = EXCLUDED.linked_at
	`, userID, chatID, username); err != nil {
		return "", fmt.Errorf("upsert telegram link: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return userID, nil
}

func (s *Store) GetTelegramLink(ctx context.Context, userID string) (TelegramLink, error) {
	var l TelegramLink
	var username sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, chat_id, username, linked_at FROM telegram_links WHERE user_id = $1
	`, userID).Scan(&l.UserID, &l.ChatID, &username, &l.LinkedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TelegramLink{}, ErrNotFound
	}
	if err != nil {
		return TelegramLink{}, fmt.Errorf("select telegram link: %w", err)
	}
	l.Username = username.String
	return l, nil
}

func (s *Store) DeleteTelegramLink(ctx context.Context, userID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM telegram_links WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete telegram link: %w", err)
	}
	return nil
}

func (s *Store) DeleteTelegramLinkByChat(ctx context.Context, chatID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM telegram_links WHERE chat_id = $1`, chatID)
	if err != nil {
		return false, fmt.Errorf("delete telegram link by chat: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
