// Command parser runs the parser worker: consumes report.uploaded tasks
// from RabbitMQ, downloads the report from MinIO, parses it, and submits
// the resulting trades to the portfolio service - now over gRPC
// (invest.portfolio.v1.PortfolioService/CreateTrade), not HTTP. parser
// itself exposes no API of its own; it only runs a gRPC health server so
// Docker/operators have something to poll. See architecture-decisions.md,
// "перевод внутреннего взаимодействия сервисов на gRPC".
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"invest/backend/services/parser/internal/config"
	"invest/backend/services/parser/internal/health"
	"invest/backend/services/parser/internal/objectstore"
	"invest/backend/services/parser/internal/parsing"
	"invest/backend/services/parser/internal/portfolioclient"
	"invest/backend/services/parser/internal/queue"
	"invest/backend/services/parser/internal/worker"
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

	consumer, err := queue.Connect(cfg.RabbitMQURL, cfg.RabbitMQQueue)
	if err != nil {
		log.Error("failed to connect to rabbitmq", "error", err)
		os.Exit(1)
	}
	defer consumer.Close()

	deliveries, err := consumer.Consume("parser")
	if err != nil {
		log.Error("failed to start consuming", "error", err)
		os.Exit(1)
	}

	portfolio, err := portfolioclient.New(cfg.PortfolioGRPCAddr)
	if err != nil {
		log.Error("failed to dial portfolio service", "error", err)
		os.Exit(1)
	}
	defer portfolio.Close()

	// The broker -> parser registry. New brokers get a concrete
	// parsing.Parser implementation and a Register line here (see
	// internal/parsing.Dispatcher). Until the first real parser exists,
	// the Stub is registered so the pipeline itself stays exercised
	// end-to-end in dev; it returns ErrNotImplemented, which the worker
	// logs and drops the task for.
	parsers := parsing.NewDispatcher()
	parsers.Register("tinkoff", parsing.Stub{})

	w := &worker.Worker{
		Store:     store,
		Parser:    parsers,
		Portfolio: portfolio,
		Log:       log,
	}
	go w.Run(ctx, deliveries)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Error("failed to listen", "addr", cfg.GRPCAddr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, &health.Server{
		Queue: consumer,
		Store: store,
		OnUnhealthy: func(err error) {
			log.Warn("health check failing", "error", err)
		},
	})
	reflection.Register(grpcServer)

	go func() {
		<-ctx.Done()
		log.Info("parser service shutting down")
		grpcServer.GracefulStop()
	}()

	log.Info("parser service starting", "addr", cfg.GRPCAddr, "queue", cfg.RabbitMQQueue, "portfolio_addr", cfg.PortfolioGRPCAddr, "brokers", parsers.SupportedBrokers())
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
	log.Info("parser service stopped")
}

// runHealthcheckClient dials addr and calls the standard gRPC health
// check as a plain client, in a separate short-lived process invocation
// of this same binary - this is what the Dockerfile's HEALTHCHECK runs.
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
