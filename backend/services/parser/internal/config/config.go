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

	S3Endpoint string

	S3AccessKey string
	S3SecretKey string

	S3UseSSL bool

	PortfolioGRPCAddr string

	LogLevel string
}

func Load() (Config, error) {
	cfg := Config{
		GRPCAddr:          getEnv("GRPC_ADDR", ":8084"),
		RabbitMQURL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQQueue:     getEnv("RABBITMQ_QUEUE", "report.uploaded"),
		S3Endpoint:        getEnv("S3_ENDPOINT", "localhost:9000"),
		S3AccessKey:       getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:       getEnv("S3_SECRET_KEY", ""),
		PortfolioGRPCAddr: getEnv("PORTFOLIO_GRPC_ADDR", "localhost:8083"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
	}

	useSSL, err := strconv.ParseBool(getEnv("S3_USE_SSL", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("S3_USE_SSL must be true or false: %w", err)
	}
	cfg.S3UseSSL = useSSL

	if cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
		return Config{}, fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
