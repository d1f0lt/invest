






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
	"invest/backend/services/parser/internal/parsing/sber"
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

	
	
	
	parsers := parsing.NewDispatcher()
	parsers.Register(sber.BrokerKey, sber.Parser{})

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
