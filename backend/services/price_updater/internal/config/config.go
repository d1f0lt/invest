package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string

	MoexISSBaseURL string

	MoexBoards []string

	PollInterval time.Duration

	HTTPTimeout time.Duration

	HistoryRunHour   int
	HistoryRunMinute int

	BackfillDays int

	HistoryRequestPause time.Duration

	LogLevel string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/invest?sslmode=disable"),
		MoexISSBaseURL: getEnv("MOEX_ISS_BASE_URL", "https://iss.moex.com/iss"),
		MoexBoards:     splitAndTrim(getEnv("MOEX_BOARDS", "TQBR,TQTF,TQOB,TQCB")),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
	}

	pollInterval, err := time.ParseDuration(getEnv("POLL_INTERVAL", "1m"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid POLL_INTERVAL: %w", err)
	}
	if pollInterval <= 0 {
		// time.NewTicker panics on a non-positive interval.
		return Config{}, fmt.Errorf("POLL_INTERVAL must be positive, got %s", pollInterval)
	}
	cfg.PollInterval = pollInterval

	httpTimeout, err := time.ParseDuration(getEnv("HTTP_TIMEOUT", "15s"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid HTTP_TIMEOUT: %w", err)
	}
	cfg.HTTPTimeout = httpTimeout

	hour, minute, err := parseClock(getEnv("HISTORY_RUN_AT", "03:00"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid HISTORY_RUN_AT: %w", err)
	}
	cfg.HistoryRunHour, cfg.HistoryRunMinute = hour, minute

	backfillDays, err := strconv.Atoi(getEnv("BACKFILL_DAYS", "365"))
	if err != nil || backfillDays < 0 {
		return Config{}, fmt.Errorf("invalid BACKFILL_DAYS: must be a non-negative integer")
	}
	cfg.BackfillDays = backfillDays

	pause, err := time.ParseDuration(getEnv("HISTORY_REQUEST_PAUSE", "200ms"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid HISTORY_REQUEST_PAUSE: %w", err)
	}
	cfg.HistoryRequestPause = pause

	if len(cfg.MoexBoards) == 0 {
		return Config{}, fmt.Errorf("MOEX_BOARDS must list at least one board id")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must not be empty")
	}

	return cfg, nil
}


func parseClock(s string) (int, int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, 0, fmt.Errorf("want HH:MM, got %q", s)
	}
	return t.Hour(), t.Minute(), nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}
