package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"invest/backend/services/gateway/internal/config"
	"invest/backend/services/gateway/internal/httpapi"
	"invest/backend/services/gateway/internal/objectstore"
	"invest/backend/services/gateway/internal/queue"
	"invest/backend/services/gateway/internal/upstream"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := objectstore.New(objectstore.Config{
		Endpoint:  cfg.MinIOEndpoint,
		AccessKey: cfg.MinIOAccessKey,
		SecretKey: cfg.MinIOSecretKey,
		UseSSL:    cfg.MinIOUseSSL,
	})
	if err != nil {
		log.Error("failed to create minio client", "error", err)
		os.Exit(1)
	}

	publisher, err := queue.Connect(cfg.RabbitMQURL, cfg.RabbitMQQueue)
	if err != nil {
		log.Error("failed to connect to rabbitmq", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()

	clients, err := upstream.Dial(upstream.Addrs{
		Users:            cfg.UsersGRPCAddr,
		Portfolio:        cfg.PortfolioGRPCAddr,
		SecuritiesReader: cfg.SecuritiesReaderGRPCAddr,
	})
	if err != nil {
		log.Error("failed to dial backend services", "error", err)
		os.Exit(1)
	}
	defer clients.Close()

	handlers := &httpapi.Handlers{
		JWTSecret: cfg.JWTSecret,

		Upstream:        clients,
		UpstreamTimeout: cfg.UpstreamTimeout,

		Queue: publisher,
		Store: store,

		Reports: &httpapi.ReportsHandler{
			Portfolio:       clients.Portfolio,
			UpstreamTimeout: cfg.UpstreamTimeout,
			Store:           store,
			Queue:           publisher,
			Bucket:          cfg.ReportsBucket,
			MaxUploadBytes:  cfg.MaxUploadBytes,
			CleanupTimeout:  5 * time.Second,
			Log:             log,
		},

		Log: log,
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewMux(handlers),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", "error", err)
		}
	}()

	log.Info("gateway service starting", "addr", cfg.HTTPAddr,
		"users", cfg.UsersGRPCAddr, "portfolio", cfg.PortfolioGRPCAddr, "securities_reader", cfg.SecuritiesReaderGRPCAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
	log.Info("gateway service stopped")
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
