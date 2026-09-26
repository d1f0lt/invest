package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var ErrImportFinished = errors.New("report import is already finished")

const (
	ImportQueued     = "queued"
	ImportProcessing = "processing"
	ImportDone       = "done"
	ImportFailed     = "failed"
)

type Broker struct {
	ID          string
	Name        string
	FileFormats []string
	IconURL     string
	Color       string
	Enabled     bool
}

type ReportImport struct {
	ID          string
	PortfolioID string
	BrokerID    string
	Filename    string
	Status      string
	Error       string

	TradesCreated, TradesSkipped int
	CashCreated, CashSkipped     int

	CreatedAt  time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time
}

func (s *Store) ListBrokers(ctx context.Context) ([]Broker, error) {
	const stmt = `
		SELECT id, name, file_formats, COALESCE(icon_url, ''), COALESCE(color, ''), enabled
		FROM brokers WHERE enabled
		ORDER BY sort_order, name
	`
	rows, err := s.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("select brokers: %w", err)
	}
	defer rows.Close()

	var out []Broker
	for rows.Next() {
		var b Broker
		if err := rows.Scan(&b.ID, &b.Name, pq.Array(&b.FileFormats), &b.IconURL, &b.Color, &b.Enabled); err != nil {
			return nil, fmt.Errorf("scan broker: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetBroker(ctx context.Context, id string) (Broker, error) {
	const stmt = `
		SELECT id, name, file_formats, COALESCE(icon_url, ''), COALESCE(color, ''), enabled
		FROM brokers WHERE id = $1
	`
	var b Broker
	err := s.db.QueryRowContext(ctx, stmt, id).Scan(&b.ID, &b.Name, pq.Array(&b.FileFormats), &b.IconURL, &b.Color, &b.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return Broker{}, ErrNotFound
	}
	if err != nil {
		return Broker{}, fmt.Errorf("select broker: %w", err)
	}
	return b, nil
}

const reportImportColumns = `
	id, portfolio_id, broker_id, filename, status, error,
	trades_created, trades_skipped, cash_operations_created, cash_operations_skipped,
	created_at, updated_at, finished_at
`

type rowScanner interface{ Scan(dest ...any) error }

func scanReportImport(row rowScanner) (ReportImport, error) {
	var r ReportImport
	var finished sql.NullTime
	err := row.Scan(&r.ID, &r.PortfolioID, &r.BrokerID, &r.Filename, &r.Status, &r.Error,
		&r.TradesCreated, &r.TradesSkipped, &r.CashCreated, &r.CashSkipped,
		&r.CreatedAt, &r.UpdatedAt, &finished)
	if finished.Valid {
		t := finished.Time
		r.FinishedAt = &t
	}
	return r, err
}

func (s *Store) CreateReportImport(ctx context.Context, r ReportImport) (ReportImport, error) {
	stmt := `
		INSERT INTO report_imports (id, portfolio_id, broker_id, filename, status)
		VALUES ($1, $2, $3, $4, 'queued')
		RETURNING ` + reportImportColumns
	out, err := scanReportImport(s.db.QueryRowContext(ctx, stmt, r.ID, r.PortfolioID, r.BrokerID, r.Filename))
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == postgresUniqueViolation {
			return ReportImport{}, fmt.Errorf("%w: report import %s", ErrDuplicate, r.ID)
		}
		return ReportImport{}, fmt.Errorf("insert report import: %w", err)
	}
	return out, nil
}

func (s *Store) GetReportImport(ctx context.Context, id string) (ReportImport, error) {
	stmt := `SELECT ` + reportImportColumns + ` FROM report_imports WHERE id = $1`
	r, err := scanReportImport(s.db.QueryRowContext(ctx, stmt, id))
	if errors.Is(err, sql.ErrNoRows) || isInvalidText(err) {
		return ReportImport{}, ErrNotFound
	}
	if err != nil {
		return ReportImport{}, fmt.Errorf("select report import: %w", err)
	}
	return r, nil
}

func (s *Store) ListReportImports(ctx context.Context, portfolioID string) ([]ReportImport, error) {
	stmt := `SELECT ` + reportImportColumns + `
		FROM report_imports WHERE portfolio_id = $1
		ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, stmt, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("select report imports: %w", err)
	}
	defer rows.Close()

	var out []ReportImport
	for rows.Next() {
		r, err := scanReportImport(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report import: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateReportImportStatus(ctx context.Context, id, status, errMsg string) (ReportImport, error) {
	stmt := `
		UPDATE report_imports
		SET status = $2, error = $3, updated_at = now(),
		    finished_at = CASE WHEN $2 IN ('done', 'failed') THEN now() END
		WHERE id = $1 AND status IN ('queued', 'processing')
		RETURNING ` + reportImportColumns
	r, err := scanReportImport(s.db.QueryRowContext(ctx, stmt, id, status, errMsg))
	if errors.Is(err, sql.ErrNoRows) {
		if _, getErr := s.GetReportImport(ctx, id); getErr != nil {
			return ReportImport{}, getErr
		}
		return ReportImport{}, ErrImportFinished
	}
	if isInvalidText(err) {
		return ReportImport{}, ErrNotFound
	}
	if err != nil {
		return ReportImport{}, fmt.Errorf("update report import: %w", err)
	}
	return r, nil
}
