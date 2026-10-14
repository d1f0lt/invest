package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type SecurityMetadata struct {
	Values map[string]string
	Issuer string
}

type Dividend struct {
	RegistryCloseDate string
	DeclaredDate      string
	Value             float64
	Currency          string
	Forecast          bool
}

func (s *Store) SecurityMetadata(ctx context.Context, secid string) (SecurityMetadata, error) {
	var data []byte
	var issuer sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT description, issuer FROM security_metadata WHERE secid=$1`, secid).Scan(&data, &issuer)
	if errors.Is(err, sql.ErrNoRows) {
		return SecurityMetadata{}, nil
	}
	if err != nil {
		return SecurityMetadata{}, fmt.Errorf("security metadata: %w", err)
	}
	out := SecurityMetadata{Issuer: issuer.String}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out.Values); err != nil {
			return SecurityMetadata{}, fmt.Errorf("decode security metadata: %w", err)
		}
	}
	return out, nil
}

func (s *Store) Dividends(ctx context.Context, secid string) ([]Dividend, bool, error) {
	var data []byte
	var updated sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT dividends, dividends_updated_at FROM security_metadata WHERE secid=$1`, secid).Scan(&data, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("dividends: %w", err)
	}
	if !updated.Valid {
		return nil, false, nil
	}
	var out []Dividend
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, false, fmt.Errorf("decode dividends: %w", err)
		}
	}
	return out, true, nil
}

func IsBondBoard(board string) bool {
	switch board {
	case "TQOB", "TQCB", "TQIR", "TQOD", "TQOE", "TQOY", "TQRD":
		return true
	default:
		return false
	}
}
