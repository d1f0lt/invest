package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"invest/backend/services/notifier/internal/bot"
	"invest/backend/services/notifier/internal/checker"
	"invest/backend/services/notifier/internal/config"
	"invest/backend/services/notifier/internal/grpcserver"
	"invest/backend/services/notifier/internal/health"
	"invest/backend/services/notifier/internal/storage"
	"invest/backend/services/notifier/internal/telegram"
	notifierpb "invest/backend/services/notifier/proto"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "dial GRPC_ADDR, run the standard gRPC health check, and exit 0/1 (Docker HEALTHCHECK)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if *healthcheck {
		os.Exit(runHealthcheckClient(cfg.GRPCAddr))
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Error("load Europe/Moscow", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := storage.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	srv := &grpcserver.Server{
		Store:            store,
		LinkTokenTTL:     cfg.LinkTokenTTL,
		MaxAlertsPerUser: cfg.MaxAlertsPerUser,
		Log:              log,
	}

	var workers sync.WaitGroup
	if cfg.TelegramBotToken != "" {
		api := telegram.New(cfg.TelegramAPIURL, cfg.TelegramBotToken)
		b := &bot.Bot{API: api, Store: store, Log: log}
		srv.Bot = b
		c := &checker.Checker{
			Store: store, Sender: api, Loc: moscow,
			Interval: cfg.CheckInterval, StaleAfter: cfg.PriceStaleAfter,
			SendPause: 50 * time.Millisecond, Log: log,
		}
		workers.Add(2)
		go func() { defer workers.Done(); b.Run(ctx) }()
		go func() { defer workers.Done(); c.Run(ctx) }()
	} else {
		log.Warn("TELEGRAM_BOT_TOKEN is not set: alerts can be managed, but nothing will be sent")
	}

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Error("failed to listen", "addr", cfg.GRPCAddr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	notifierpb.RegisterNotifierServiceServer(grpcServer, srv)
	grpc_health_v1.RegisterHealthServer(grpcServer, &health.Server{
		Probe: func(ctx context.Context) error { return store.Ping(ctx) },
	})
	reflection.Register(grpcServer)

	go func() {
		<-ctx.Done()
		log.Info("notifier service shutting down")
		grpcServer.GracefulStop()
	}()

	log.Info("notifier service starting", "addr", cfg.GRPCAddr, "telegram", cfg.TelegramBotToken != "")
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
	workers.Wait()
	log.Info("notifier service stopped")
}

func runHealthcheckClient(addr string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck: dial:", err)
		return 1
	}
	defer conn.Close()

	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck: check:", err)
		return 1
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		fmt.Fprintln(os.Stderr, "healthcheck: not serving:", resp.GetStatus())
		return 1
	}
	return 0
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
