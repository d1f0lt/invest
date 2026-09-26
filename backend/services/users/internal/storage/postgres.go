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
	ErrNotFound             = errors.New("user not found")
	ErrEmailTaken           = errors.New("email already registered")
	ErrUsernameTaken        = errors.New("username already taken")
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
)

const postgresUniqueViolation = "23505"

type User struct {
	ID           string
	Email        string
	Username     *string
	PasswordHash string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}

type RefreshToken struct {
	ID        string
	UserID    string
	FamilyID  string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
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

func (s *Store) CreateUser(ctx context.Context, email string, username *string, passwordHash string) (User, error) {
	const stmt = `
		INSERT INTO users (email, username, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, username, is_active, created_at, updated_at, last_login_at
	`

	var u User
	err := s.db.QueryRowContext(ctx, stmt, email, username, passwordHash).Scan(
		&u.ID, &u.Email, &u.Username, &u.IsActive, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == postgresUniqueViolation {
			if pqErr.Constraint == "idx_users_username_lower" {
				return User{}, ErrUsernameTaken
			}
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	const stmt = `
		SELECT id, email, username, is_active, created_at, updated_at, last_login_at
		FROM users WHERE id = $1
	`
	var u User
	err := s.db.QueryRowContext(ctx, stmt, id).Scan(
		&u.ID, &u.Email, &u.Username, &u.IsActive, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("select user by id: %w", err)
	}
	return u, nil
}

func (s *Store) GetUserByEmailWithPassword(ctx context.Context, email string) (User, error) {
	const stmt = `
		SELECT id, email, username, password_hash, is_active, created_at, updated_at, last_login_at
		FROM users WHERE LOWER(email) = LOWER($1)
	`
	var u User
	err := s.db.QueryRowContext(ctx, stmt, email).Scan(
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.IsActive, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("select user by email: %w", err)
	}
	return u, nil
}

func (s *Store) TouchLastLogin(ctx context.Context, id string) error {
	const stmt = `UPDATE users SET last_login_at = now() WHERE id = $1`
	_, err := s.db.ExecContext(ctx, stmt, id)
	if err != nil {
		return fmt.Errorf("touch last_login_at: %w", err)
	}
	return nil
}

func (s *Store) CreateRefreshToken(ctx context.Context, userID, familyID, tokenHash string, expiresAt time.Time) (RefreshToken, error) {

	const stmt = `
		INSERT INTO refresh_tokens (user_id, family_id, token_hash, expires_at)
		VALUES ($1, COALESCE(NULLIF($2::text, '')::uuid, gen_random_uuid()), $3, $4)
		RETURNING id, user_id, family_id, token_hash, expires_at, revoked_at, created_at
	`
	var rt RefreshToken
	err := s.db.QueryRowContext(ctx, stmt, userID, familyID, tokenHash, expiresAt).Scan(
		&rt.ID, &rt.UserID, &rt.FamilyID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.CreatedAt,
	)
	if err != nil {
		return RefreshToken{}, fmt.Errorf("insert refresh token: %w", err)
	}
	return rt, nil
}

func (s *Store) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	const stmt = `
		SELECT id, user_id, family_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1
	`
	var rt RefreshToken
	err := s.db.QueryRowContext(ctx, stmt, tokenHash).Scan(
		&rt.ID, &rt.UserID, &rt.FamilyID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RefreshToken{}, ErrRefreshTokenNotFound
	}
	if err != nil {
		return RefreshToken{}, fmt.Errorf("select refresh token by hash: %w", err)
	}
	return rt, nil
}

func (s *Store) ClaimRefreshToken(ctx context.Context, id string) (bool, error) {
	const stmt = `UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL AND expires_at > now()`
	res, err := s.db.ExecContext(ctx, stmt, id)
	if err != nil {
		return false, fmt.Errorf("claim refresh token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim refresh token: rows affected: %w", err)
	}
	return n == 1, nil
}

func (s *Store) RevokeRefreshTokenFamily(ctx context.Context, familyID string) error {
	const stmt = `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`
	_, err := s.db.ExecContext(ctx, stmt, familyID)
	if err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}

func (s *Store) RevokeRefreshTokenByHash(ctx context.Context, tokenHash string) error {
	const stmt = `UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`
	_, err := s.db.ExecContext(ctx, stmt, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke refresh token by hash: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredRefreshTokens(ctx context.Context, cutoff time.Time) (int64, error) {
	const stmt = `DELETE FROM refresh_tokens WHERE expires_at < $1`
	res, err := s.db.ExecContext(ctx, stmt, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired refresh tokens: rows affected: %w", err)
	}
	return n, nil
}
