package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"invest/backend/services/feed/internal/grpcserver"
	feedpb "invest/backend/services/feed/proto"
)

func main() {
	probe := flag.Bool("healthcheck", false, "check the local feed service")
	flag.Parse()
	addr := os.Getenv("GRPC_ADDR")
	if addr == "" {
		addr = ":8086"
	}
	if *probe {
		if err := checkHealth(addr); err != nil {
			slog.Error("feed healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Error("DATABASE_URL must be set")
		os.Exit(1)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Error("failed to configure feed database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error("failed to listen", "addr", addr, "error", err)
		os.Exit(1)
	}

	server := grpc.NewServer()
	feedpb.RegisterFeedServiceServer(server, &grpcserver.Server{DB: db, Log: log})
	grpc_health_v1.RegisterHealthServer(server, &feedHealth{db: db})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()

	log.Info("feed service starting", "addr", addr)
	select {
	case err := <-done:
		if err != nil {
			log.Error("feed server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		timeout := time.AfterFunc(10*time.Second, server.Stop)
		server.GracefulStop()
		timeout.Stop()
	}
}

type feedHealth struct {
	grpc_health_v1.UnimplementedHealthServer
	db *sql.DB
}

func (h *feedHealth) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	_, err := h.db.ExecContext(ctx, `SELECT feed_posts.forecast_revised_at FROM feed_posts, feed_author_scores, feed_comments, feed_reactions, users WHERE users.is_admin LIMIT 0`)
	state := grpc_health_v1.HealthCheckResponse_SERVING
	if err != nil {
		state = grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}
	return &grpc_health_v1.HealthCheckResponse{Status: state}, nil
}

func checkHealth(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	conn, err := grpc.NewClient(net.JoinHostPort(host, port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return errors.New("feed is not serving")
	}
	return nil
}
