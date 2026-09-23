// Package health implements the gRPC health-checking protocol
// (grpc.health.v1.Health) around a single synchronous probe function,
// called fresh on every Check RPC.
//
// This is deliberately not the stock google.golang.org/grpc/health.Server:
// that implementation only ever reports whatever status was last pushed
// to it via SetServingStatus (a push model, meant for a background
// goroutine to update periodically). This package re-runs Probe
// synchronously on every incoming Check call instead, which matches what
// this project's old HTTP GET /healthz endpoint did - a health check
// that reflects the current state of the world, not a cached one.
package health

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Probe reports whether the service is currently healthy.
type Probe func(ctx context.Context) error

// Server implements grpc_health_v1.HealthServer around a single Probe.
type Server struct {
	grpc_health_v1.UnimplementedHealthServer
	Probe Probe
}

// Check implements the unary health RPC. The "service" field of the
// request is ignored - this project has exactly one health signal per
// process, same as the old single /healthz endpoint per service.
func (s *Server) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if err := s.Probe(ctx); err != nil {
		return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, nil
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

// Watch is not implemented - nothing in this project streams health
// status, every caller (docker healthcheck, gateway's own /healthz)
// polls with a plain Check.
func (s *Server) Watch(_ *grpc_health_v1.HealthCheckRequest, _ grpc_health_v1.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "watch is not implemented; call Check")
}
