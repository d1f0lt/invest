package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string

	// GRPCAddr is the address the gRPC server listens on, e.g. ":8083".
	// Renamed 2026-09-20 from HTTPAddr/HTTP_ADDR: this service no longer
	// speaks HTTP at all - see architecture-decisions.md, "перевод
	// внутреннего взаимодействия сервисов на gRPC".
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
