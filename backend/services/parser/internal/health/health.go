















package health

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)



type Pinger interface {
	Ping(ctx context.Context) error
}



type Server struct {
	grpc_health_v1.UnimplementedHealthServer
	Queue Pinger
	Store Pinger
	
	
	
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
