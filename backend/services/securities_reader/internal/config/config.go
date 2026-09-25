package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL string

	GRPCAddr string

	LogLevel string

	MoexISSBaseURL string
	MoexTimeout    time.Duration

	DividendsBaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/invest?sslmode=disable"),
		GRPCAddr:    getEnv("GRPC_ADDR", ":8081"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),

		MoexISSBaseURL: getEnv("MOEX_ISS_BASE_URL", "https://iss.moex.com/iss"),

		DividendsBaseURL: getEnv("DIVIDENDS_BASE_URL", "https://www.dohod.ru/ik/analytics/dividend"),
	}

	timeout, err := time.ParseDuration(getEnv("MOEX_TIMEOUT", "10s"))
	if err != nil || timeout <= 0 {
		return Config{}, fmt.Errorf("MOEX_TIMEOUT must be a positive duration")
	}
	cfg.MoexTimeout = timeout

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
