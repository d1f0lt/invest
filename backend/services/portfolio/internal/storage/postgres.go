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
	ErrNotFound          = errors.New("not found")
	ErrUnknownInstrument = errors.New("unknown instrument: no such secid/board in securities")
	
	
	ErrDuplicate = errors.New("duplicate external_id")
)

const (
	postgresForeignKeyViolation = "23503"
	postgresUniqueViolation     = "23505"
	
	postgresInvalidTextRepresentation = "22P02"
)



func isInvalidText(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == postgresInvalidTextRepresentation
}




func mapInsertErr(err error, t Trade) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case postgresForeignKeyViolation:
			return fmt.Errorf("%w: %s/%s", ErrUnknownInstrument, t.SecID, t.Board)
		case postgresUniqueViolation:
			return fmt.Errorf("%w: %s", ErrDuplicate, t.ExternalID)
		}
	}
	return fmt.Errorf("insert trade: %w", err)
}

type Portfolio struct {
	ID        string
	UserID    string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Trade struct {
	ID          string
	PortfolioID string
	SecID       string
	Board       string
	Side        string
	Quantity    float64
	Price       float64
	Fee         float64
	Currency    string
	ExecutedAt  time.Time
	CreatedAt   time.Time

	AccruedInterest float64
	
	ExternalID string
}

type CashOperation struct {
	ID          string
	PortfolioID string
	Type        string
	Amount      float64
	Currency    string
	OccurredAt  time.Time
	SecID       string 
	Board       string
	Description string
	ExternalID  string
	CreatedAt   time.Time
}



type ImportResult struct {
	TradesCreated, TradesSkipped int
	CashCreated, CashSkipped     int
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

func (s *Store) Close() { s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) CreatePortfolio(ctx context.Context, userID, name string) (Portfolio, error) {
	const stmt = `
		INSERT INTO portfolios (user_id, name)
		VALUES ($1, $2)
		RETURNING id, user_id, name, created_at, updated_at
	`
	var p Portfolio
	err := s.db.QueryRowContext(ctx, stmt, userID, name).Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Portfolio{}, fmt.Errorf("insert portfolio: %w", err)
	}
	return p, nil
}

func (s *Store) ListPortfoliosByUser(ctx context.Context, userID string) ([]Portfolio, error) {
	const stmt = `
		SELECT id, user_id, name, created_at, updated_at
		FROM portfolios WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.db.QueryContext(ctx, stmt, userID)
	if err != nil {
		return nil, fmt.Errorf("select portfolios: %w", err)
	}
	defer rows.Close()

	var out []Portfolio
	for rows.Next() {
		var p Portfolio
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan portfolio: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPortfolio(ctx context.Context, id string) (Portfolio, error) {
	const stmt = `
		SELECT id, user_id, name, created_at, updated_at
		FROM portfolios WHERE id = $1
	`
	var p Portfolio
	err := s.db.QueryRowContext(ctx, stmt, id).Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	
	
	if errors.Is(err, sql.ErrNoRows) || isInvalidText(err) {
		return Portfolio{}, ErrNotFound
	}
	if err != nil {
		return Portfolio{}, fmt.Errorf("select portfolio: %w", err)
	}
	return p, nil
}



func (s *Store) RenamePortfolio(ctx context.Context, id, name string) (Portfolio, error) {
	const stmt = `
		UPDATE portfolios SET name = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, user_id, name, created_at, updated_at
	`
	var p Portfolio
	err := s.db.QueryRowContext(ctx, stmt, id, name).Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) || isInvalidText(err) {
		return Portfolio{}, ErrNotFound
	}
	if err != nil {
		return Portfolio{}, fmt.Errorf("update portfolio: %w", err)
	}
	return p, nil
}

func (s *Store) CreateTrade(ctx context.Context, t Trade) (Trade, error) {
	const stmt = `
		INSERT INTO trades (portfolio_id, secid, board, side, quantity, price, fee, currency, executed_at, accrued_interest, external_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''))
		RETURNING id, portfolio_id, secid, board, side, quantity, price, fee, currency, executed_at, created_at, accrued_interest, COALESCE(external_id, '')
	`
	var out Trade
	err := s.db.QueryRowContext(ctx, stmt,
		t.PortfolioID, t.SecID, t.Board, t.Side, t.Quantity, t.Price, t.Fee, t.Currency, t.ExecutedAt,
		t.AccruedInterest, t.ExternalID,
	).Scan(
		&out.ID, &out.PortfolioID, &out.SecID, &out.Board, &out.Side,
		&out.Quantity, &out.Price, &out.Fee, &out.Currency, &out.ExecutedAt, &out.CreatedAt,
		&out.AccruedInterest, &out.ExternalID,
	)
	if err != nil {
		return Trade{}, mapInsertErr(err, t)
	}
	return out, nil
}







func (s *Store) CreateTradesBatch(ctx context.Context, trades []Trade) ([]Trade, error) {
	if len(trades) == 0 {
		return nil, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	const stmt = `
		INSERT INTO trades (portfolio_id, secid, board, side, quantity, price, fee, currency, executed_at, accrued_interest, external_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''))
		RETURNING id, portfolio_id, secid, board, side, quantity, price, fee, currency, executed_at, created_at, accrued_interest, COALESCE(external_id, '')
	`

	out := make([]Trade, 0, len(trades))
	for _, t := range trades {
		var created Trade
		err := tx.QueryRowContext(ctx, stmt,
			t.PortfolioID, t.SecID, t.Board, t.Side, t.Quantity, t.Price, t.Fee, t.Currency, t.ExecutedAt,
			t.AccruedInterest, t.ExternalID,
		).Scan(
			&created.ID, &created.PortfolioID, &created.SecID, &created.Board, &created.Side,
			&created.Quantity, &created.Price, &created.Fee, &created.Currency, &created.ExecutedAt, &created.CreatedAt,
			&created.AccruedInterest, &created.ExternalID,
		)
		if err != nil {
			return nil, mapInsertErr(err, t)
		}
		out = append(out, created)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit batch: %w", err)
	}
	tx = nil
	return out, nil
}

func (s *Store) ListTrades(ctx context.Context, portfolioID string) ([]Trade, error) {
	const stmt = `
		SELECT id, portfolio_id, secid, board, side, quantity, price, fee, currency, executed_at, created_at, accrued_interest, COALESCE(external_id, '')
		FROM trades WHERE portfolio_id = $1
		ORDER BY executed_at ASC, created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, stmt, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("select trades: %w", err)
	}
	defer rows.Close()

	var out []Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(
			&t.ID, &t.PortfolioID, &t.SecID, &t.Board, &t.Side,
			&t.Quantity, &t.Price, &t.Fee, &t.Currency, &t.ExecutedAt, &t.CreatedAt,
			&t.AccruedInterest, &t.ExternalID,
		); err != nil {
			return nil, fmt.Errorf("scan trade: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) LatestPrices(ctx context.Context, instruments [][2]string) (map[string]float64, error) {
	out := make(map[string]float64, len(instruments))
	if len(instruments) == 0 {
		return out, nil
	}

	secids := make(pq.StringArray, len(instruments))
	boards := make(pq.StringArray, len(instruments))
	for i, inst := range instruments {
		secids[i], boards[i] = inst[0], inst[1]
	}

	
	
	
	
	const stmt = `
		SELECT lp.secid, lp.board,
		       CASE WHEN s.price_in_percent AND s.face_value IS NOT NULL
		            THEN lp.last_price * s.face_value / 100
		            ELSE lp.last_price
		       END
		FROM latest_prices lp
		JOIN securities s ON s.secid = lp.secid AND s.board = lp.board
		WHERE (lp.secid, lp.board) IN (
			SELECT * FROM unnest($1::text[], $2::text[])
		)
		AND lp.last_price IS NOT NULL
	`
	rows, err := s.db.QueryContext(ctx, stmt, secids, boards)
	if err != nil {
		return nil, fmt.Errorf("select latest prices: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var secid, board string
		var price float64
		if err := rows.Scan(&secid, &board, &price); err != nil {
			return nil, fmt.Errorf("scan latest price: %w", err)
		}
		out[secid+"/"+board] = price
	}
	return out, rows.Err()
}













func (s *Store) ImportReport(ctx context.Context, portfolioID, importID string, trades []Trade, cash []CashOperation) (ImportResult, error) {
	var res ImportResult

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	const tradeStmt = `
		INSERT INTO trades (portfolio_id, secid, board, side, quantity, price, fee, currency, executed_at, accrued_interest, external_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''))
		ON CONFLICT (portfolio_id, external_id) WHERE external_id IS NOT NULL DO NOTHING
		RETURNING id
	`
	for _, t := range trades {
		var id string
		err := tx.QueryRowContext(ctx, tradeStmt,
			portfolioID, t.SecID, t.Board, t.Side, t.Quantity, t.Price, t.Fee, t.Currency, t.ExecutedAt,
			t.AccruedInterest, t.ExternalID,
		).Scan(&id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			res.TradesSkipped++
		case err != nil:
			return ImportResult{}, mapInsertErr(err, t)
		default:
			res.TradesCreated++
		}
	}

	const cashStmt = `
		INSERT INTO cash_operations (portfolio_id, type, amount, currency, occurred_at, secid, board, description, external_id)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, NULLIF($9, ''))
		ON CONFLICT (portfolio_id, external_id) WHERE external_id IS NOT NULL DO NOTHING
		RETURNING id
	`
	for _, c := range cash {
		var id string
		err := tx.QueryRowContext(ctx, cashStmt,
			portfolioID, c.Type, c.Amount, c.Currency, c.OccurredAt, c.SecID, c.Board, c.Description, c.ExternalID,
		).Scan(&id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			res.CashSkipped++
		case err != nil:
			return ImportResult{}, fmt.Errorf("insert cash operation: %w", err)
		default:
			res.CashCreated++
		}
	}

	if importID != "" {
		const doneStmt = `
			UPDATE report_imports
			SET status = 'done', error = '',
			    trades_created = $3, trades_skipped = $4,
			    cash_operations_created = $5, cash_operations_skipped = $6,
			    updated_at = now(), finished_at = now()
			WHERE id = $1 AND portfolio_id = $2 AND status IN ('queued', 'processing')
		`
		if _, err := tx.ExecContext(ctx, doneStmt, importID, portfolioID,
			res.TradesCreated, res.TradesSkipped, res.CashCreated, res.CashSkipped); err != nil && !isInvalidText(err) {
			return ImportResult{}, fmt.Errorf("mark report import done: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("commit import: %w", err)
	}
	tx = nil
	return res, nil
}

func (s *Store) ListCashOperations(ctx context.Context, portfolioID string) ([]CashOperation, error) {
	const stmt = `
		SELECT id, portfolio_id, type, amount, currency, occurred_at,
		       COALESCE(secid, ''), COALESCE(board, ''), description, COALESCE(external_id, ''), created_at
		FROM cash_operations WHERE portfolio_id = $1
		ORDER BY occurred_at ASC, created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, stmt, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("select cash operations: %w", err)
	}
	defer rows.Close()

	var out []CashOperation
	for rows.Next() {
		var c CashOperation
		if err := rows.Scan(
			&c.ID, &c.PortfolioID, &c.Type, &c.Amount, &c.Currency, &c.OccurredAt,
			&c.SecID, &c.Board, &c.Description, &c.ExternalID, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan cash operation: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
