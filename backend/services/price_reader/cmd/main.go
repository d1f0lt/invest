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
	"syscall"
	"time"
	_ "time/tzdata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"invest/backend/services/price_reader/internal/config"
	"invest/backend/services/price_reader/internal/grpcserver"
	"invest/backend/services/price_reader/internal/health"
	"invest/backend/services/price_reader/internal/storage"
	pricereaderpb "invest/backend/services/price_reader/proto"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "dial GRPC_ADDR, run the standard gRPC health check, and exit 0/1 - used as this image's Docker HEALTHCHECK instead of curl/wget, which have nothing to speak gRPC with")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := storage.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Error("failed to listen", "addr", cfg.GRPCAddr, "error", err)
		os.Exit(1)
	}

	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Error("failed to load Europe/Moscow time zone", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()

	pricereaderpb.RegisterPriceReaderServiceServer(grpcServer, &grpcserver.Server{
		Store: store,
		Log:   log,
		Loc:   moscow,
	})
	grpc_health_v1.RegisterHealthServer(grpcServer, &health.Server{
		Probe: func(ctx context.Context) error { return store.Ping(ctx) },
	})

	reflection.Register(grpcServer)

	go func() {
		<-ctx.Done()
		log.Info("price_reader shutting down")
		grpcServer.GracefulStop()
	}()

	log.Info("price_reader starting", "addr", cfg.GRPCAddr)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
	log.Info("price_reader stopped")
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
