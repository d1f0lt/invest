package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	"invest/backend/services/price_updater/internal/dohod"
)

type MetadataTarget struct {
	SecID        string
	InfoDue      bool
	DividendsDue bool
}

func (s *Store) MetadataTargets(ctx context.Context, boards []string) ([]MetadataTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
  SELECT s.secid,
   m.info_updated_at IS NULL OR m.info_updated_at < now()-interval '12 hours',
   bool_or(NOT s.price_in_percent) AND (m.dividends_updated_at IS NULL OR m.dividends_updated_at < now()-interval '24 hours')
  FROM securities s LEFT JOIN security_metadata m USING(secid)
  WHERE s.board=ANY($1)
  GROUP BY s.secid,m.info_updated_at,m.dividends_updated_at
  HAVING m.info_updated_at IS NULL OR m.info_updated_at < now()-interval '12 hours'
   OR (bool_or(NOT s.price_in_percent) AND (m.dividends_updated_at IS NULL OR m.dividends_updated_at < now()-interval '24 hours'))
  ORDER BY s.secid
 `, pq.Array(boards))
	if err != nil {
		return nil, fmt.Errorf("metadata targets: %w", err)
	}
	defer rows.Close()
	var out []MetadataTarget
	for rows.Next() {
		var target MetadataTarget
		if err := rows.Scan(&target.SecID, &target.InfoDue, &target.DividendsDue); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func (s *Store) SaveSecurityMetadata(ctx context.Context, secid string, values map[string]string, issuer string) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO security_metadata(secid,description,issuer,info_updated_at)
 VALUES($1,$2::jsonb,$3,now()) ON CONFLICT(secid) DO UPDATE SET description=EXCLUDED.description,issuer=EXCLUDED.issuer,info_updated_at=now()`, secid, string(data), issuer)
	return err
}

func (s *Store) SaveDividends(ctx context.Context, secid string, dividends []dohod.Dividend) error {
	if dividends == nil {
		dividends = []dohod.Dividend{}
	}
	data, err := json.Marshal(dividends)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO security_metadata(secid,dividends,dividends_updated_at)
 VALUES($1,$2::jsonb,now()) ON CONFLICT(secid) DO UPDATE SET dividends=EXCLUDED.dividends,dividends_updated_at=now()`, secid, string(data))
	return err
}
