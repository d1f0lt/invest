package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL string

	GRPCAddr string

	JWTSecret []byte

	AccessTokenTTL time.Duration

	RefreshTokenTTL time.Duration

	// CleanupInterval / CleanupRetention configure the background job that
	// deletes expired rows from refresh_tokens (internal/cleanup). Rows
	// live for expires_at + CleanupRetention, so reuse detection keeps its
	// evidence for a while after expiry.
	CleanupInterval  time.Duration
	CleanupRetention time.Duration

	BcryptCost int

	LogLevel string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/invest?sslmode=disable"),
		GRPCAddr:    getEnv("GRPC_ADDR", ":8082"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}

	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be set and at least 32 bytes long (got %d); generate one with e.g. `openssl rand -base64 32`", len(secret))
	}
	cfg.JWTSecret = []byte(secret)

	ttl, err := time.ParseDuration(getEnv("ACCESS_TOKEN_TTL", "30m"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ACCESS_TOKEN_TTL: %w", err)
	}
	cfg.AccessTokenTTL = ttl

	refreshTTL, err := time.ParseDuration(getEnv("REFRESH_TOKEN_TTL", "720h"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid REFRESH_TOKEN_TTL: %w", err)
	}
	cfg.RefreshTokenTTL = refreshTTL

	cleanupInterval, err := time.ParseDuration(getEnv("REFRESH_TOKEN_CLEANUP_INTERVAL", "1h"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid REFRESH_TOKEN_CLEANUP_INTERVAL: %w", err)
	}
	if cleanupInterval <= 0 {
		return Config{}, fmt.Errorf("REFRESH_TOKEN_CLEANUP_INTERVAL must be positive")
	}
	cfg.CleanupInterval = cleanupInterval

	cleanupRetention, err := time.ParseDuration(getEnv("REFRESH_TOKEN_CLEANUP_RETENTION", "720h"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid REFRESH_TOKEN_CLEANUP_RETENTION: %w", err)
	}
	if cleanupRetention < 0 {
		return Config{}, fmt.Errorf("REFRESH_TOKEN_CLEANUP_RETENTION must not be negative")
	}
	cfg.CleanupRetention = cleanupRetention

	cost, err := parseIntEnv("BCRYPT_COST", 12)
	if err != nil {
		return Config{}, err
	}
	cfg.BcryptCost = cost

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must not be empty")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func parseIntEnv(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}
