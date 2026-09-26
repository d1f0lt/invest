package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string

	GRPCAddr string

	LogLevel string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/invest?sslmode=disable"),
		GRPCAddr:    getEnv("GRPC_ADDR", ":8083"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}

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
