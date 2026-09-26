package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	GRPCAddr string

	RabbitMQURL string

	RabbitMQQueue string

	MinIOEndpoint string

	MinIOAccessKey string
	MinIOSecretKey string

	MinIOUseSSL bool

	PortfolioGRPCAddr string

	LogLevel string
}

func Load() (Config, error) {
	cfg := Config{
		GRPCAddr:          getEnv("GRPC_ADDR", ":8084"),
		RabbitMQURL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQQueue:     getEnv("RABBITMQ_QUEUE", "report.uploaded"),
		MinIOEndpoint:     getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:    getEnv("MINIO_ACCESS_KEY", ""),
		MinIOSecretKey:    getEnv("MINIO_SECRET_KEY", ""),
		PortfolioGRPCAddr: getEnv("PORTFOLIO_GRPC_ADDR", "localhost:8083"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
	}

	useSSL, err := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("MINIO_USE_SSL must be true or false: %w", err)
	}
	cfg.MinIOUseSSL = useSSL

	if cfg.MinIOAccessKey == "" || cfg.MinIOSecretKey == "" {
		return Config{}, fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY must be set")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
