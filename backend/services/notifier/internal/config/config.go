package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL string
	GRPCAddr    string
	LogLevel    string

	TelegramBotToken string
	TelegramAPIURL   string

	CheckInterval   time.Duration
	PriceStaleAfter time.Duration
	LinkTokenTTL    time.Duration

	MaxAlertsPerUser int
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/invest?sslmode=disable"),
		GRPCAddr:         getEnv("GRPC_ADDR", ":8085"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramAPIURL:   getEnv("TELEGRAM_API_URL", "https://api.telegram.org"),
	}

	var err error
	if cfg.CheckInterval, err = positiveDuration("CHECK_INTERVAL", "1m"); err != nil {
		return Config{}, err
	}
	if cfg.PriceStaleAfter, err = positiveDuration("PRICE_STALE_AFTER", "5m"); err != nil {
		return Config{}, err
	}
	if cfg.LinkTokenTTL, err = positiveDuration("LINK_TOKEN_TTL", "15m"); err != nil {
		return Config{}, err
	}

	maxAlerts, err := strconv.Atoi(getEnv("MAX_ALERTS_PER_USER", "100"))
	if err != nil || maxAlerts <= 0 {
		return Config{}, fmt.Errorf("MAX_ALERTS_PER_USER must be a positive integer")
	}
	cfg.MaxAlertsPerUser = maxAlerts

	return cfg, nil
}

func positiveDuration(key, fallback string) (time.Duration, error) {
	d, err := time.ParseDuration(getEnv(key, fallback))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", key, d)
	}
	return d, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
