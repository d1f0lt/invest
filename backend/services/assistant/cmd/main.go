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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"invest/backend/services/assistant/internal/chat"
	feedpb "invest/backend/services/assistant/internal/feedpb"
	portfoliopb "invest/backend/services/assistant/internal/portfoliopb"
)

func main() {
	if err := run(); err != nil {
		slog.Error("assistant failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	model, err := chat.NewAssistantModel(os.Getenv("ASSISTANT_URL"), os.Getenv("ASSISTANT_MODEL"), os.Getenv("ASSISTANT_API_KEY"))
	if err != nil {
		return err
	}
	feed, err := grpc.NewClient(env("FEED_GRPC_ADDR", "localhost:8086"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer feed.Close()
	portfolio, err := grpc.NewClient(env("PORTFOLIO_GRPC_ADDR", "localhost:8083"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer portfolio.Close()

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/assistant/chat", &chat.Handler{
		Model: model, JWTSecret: []byte(secret), Log: slog.Default(),
		Feed: feedpb.NewFeedServiceClient(feed), Portfolio: portfoliopb.NewPortfolioServiceClient(portfolio),
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr: env("HTTP_ADDR", ":8087"), Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 140 * time.Second, IdleTimeout: time.Minute,
	}
	finished := make(chan error, 1)
	go func() { finished <- server.ListenAndServe() }()
	select {
	case err := <-finished:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
