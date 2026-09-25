package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr string

	JWTSecret []byte

	UsersGRPCAddr            string
	PortfolioGRPCAddr        string
	SecuritiesReaderGRPCAddr string

	UpstreamTimeout time.Duration

	RabbitMQURL   string
	RabbitMQQueue string

	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOUseSSL    bool

	ReportsBucket string

	MaxUploadBytes int64

	LogLevel string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:                 getEnv("HTTP_ADDR", ":8080"),
		UsersGRPCAddr:            getEnv("USERS_GRPC_ADDR", "localhost:8082"),
		PortfolioGRPCAddr:        getEnv("PORTFOLIO_GRPC_ADDR", "localhost:8083"),
		SecuritiesReaderGRPCAddr: getEnv("SECURITIES_READER_GRPC_ADDR", "localhost:8081"),
		RabbitMQURL:              getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQQueue:            getEnv("RABBITMQ_QUEUE", "report.uploaded"),
		MinIOEndpoint:            getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:           getEnv("MINIO_ACCESS_KEY", ""),
		MinIOSecretKey:           getEnv("MINIO_SECRET_KEY", ""),
		ReportsBucket:            getEnv("REPORTS_BUCKET", "reports"),
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
	}

	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be set and at least 32 bytes long (got %d); it must be the SAME value the users service signs tokens with", len(secret))
	}
	cfg.JWTSecret = []byte(secret)

	timeout, err := time.ParseDuration(getEnv("UPSTREAM_TIMEOUT", "15s"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid UPSTREAM_TIMEOUT: %w", err)
	}
	cfg.UpstreamTimeout = timeout

	useSSL, err := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("MINIO_USE_SSL must be true or false: %w", err)
	}
	cfg.MinIOUseSSL = useSSL

	if cfg.MinIOAccessKey == "" || cfg.MinIOSecretKey == "" {
		return Config{}, fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY must be set")
	}

	maxUpload, err := parseSizeEnv("MAX_UPLOAD_BYTES", 32<<20)
	if err != nil {
		return Config{}, err
	}
	cfg.MaxUploadBytes = maxUpload

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func parseSizeEnv(key string, fallback int64) (int64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return n, nil
}
