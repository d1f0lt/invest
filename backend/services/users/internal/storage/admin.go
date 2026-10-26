package storage

import "context"

func (s *Store) BootstrapAdmin(ctx context.Context, hash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(736104, 4)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (email, username, password_hash, is_admin)
		SELECT 'admin@invest.local', 'admin', $1, true
		WHERE NOT EXISTS (SELECT 1 FROM users WHERE is_admin)
	`, hash); err != nil {
		return err
	}
	return tx.Commit()
}
