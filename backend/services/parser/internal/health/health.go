// Package health implements the gRPC health-checking protocol
// (grpc.health.v1.Health) for the parser service, replacing the old
// HTTP GET /healthz (internal/httpapi, deleted 2026-09-20 - see
// architecture-decisions.md, "перевод внутреннего взаимодействия
// сервисов на gRPC"). parser has no gRPC API of its own (it only
// consumes RabbitMQ and calls out to portfolio) - this is the one piece
// of it that still needs a listener at all, purely so Docker/operators
// have something to poll.
//
// Deliberately not the stock google.golang.org/grpc/health.Server (push
// model, cached status) - see the same design note in
// backend/services/users/internal/health for the full rationale. Checks
// both dependencies the old handler checked (RabbitMQ, MinIO) and, unlike
// the old handler, doesn't expose which one failed in the RPC response
// itself (grpc.health.v1.HealthCheckResponse has no message field) -
// that detail is still logged server-side (see cmd/parser/main.go).
package health

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Pinger is satisfied by both internal/queue.Consumer and
// internal/objectstore.Store.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Server implements grpc_health_v1.HealthServer by pinging both Queue
// and Store fresh on every Check call.
type Server struct {
	grpc_health_v1.UnimplementedHealthServer
	Queue Pinger
	Store Pinger
	// OnUnhealthy, if set, is called with the combined ping errors
	// whenever Check reports NOT_SERVING - used to log which dependency
	// failed, since the gRPC health response itself carries no detail.
	OnUnhealthy func(error)
}

func (s *Server) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	var errs []error
	if err := s.Queue.Ping(ctx); err != nil {
		errs = append(errs, errors.New("rabbitmq: "+err.Error()))
	}
	if err := s.Store.Ping(ctx); err != nil {
		errs = append(errs, errors.New("minio: "+err.Error()))
	}
	if len(errs) > 0 {
		if s.OnUnhealthy != nil {
			s.OnUnhealthy(errors.Join(errs...))
		}
		return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, nil
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (s *Server) Watch(_ *grpc_health_v1.HealthCheckRequest, _ grpc_health_v1.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "watch is not implemented; call Check")
}
