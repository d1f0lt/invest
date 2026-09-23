package health

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/health/grpc_health_v1"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestCheck_AllHealthy(t *testing.T) {
	s := &Server{Queue: fakePinger{}, Store: fakePinger{}}
	resp, err := s.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.GetStatus())
	}
}

func TestCheck_QueueDownReportsNotServingAndCallsOnUnhealthy(t *testing.T) {
	var reported error
	s := &Server{
		Queue:       fakePinger{err: errors.New("boom")},
		Store:       fakePinger{},
		OnUnhealthy: func(err error) { reported = err },
	}
	resp, err := s.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("status = %v, want NOT_SERVING", resp.GetStatus())
	}
	if reported == nil {
		t.Error("OnUnhealthy was not called")
	}
}

func TestCheck_BothDown(t *testing.T) {
	s := &Server{
		Queue: fakePinger{err: errors.New("rabbit down")},
		Store: fakePinger{err: errors.New("minio down")},
	}
	resp, err := s.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("status = %v, want NOT_SERVING", resp.GetStatus())
	}
}
