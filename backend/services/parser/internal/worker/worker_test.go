package worker

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsPermanent(t *testing.T) {
	cases := map[error]bool{
		status.Error(codes.FailedPrecondition, "unknown secid"):                true,
		status.Error(codes.NotFound, "portfolio not found"):                    true,
		status.Error(codes.InvalidArgument, "bad"):                             true,
		status.Error(codes.Unavailable, "down"):                                false,
		status.Error(codes.DeadlineExceeded, "slow"):                           false,
		fmt.Errorf("wrapped: %w", status.Error(codes.FailedPrecondition, "x")): true,
		errors.New("plain"): false,
	}
	for err, want := range cases {
		if got := isPermanent(err); got != want {
			t.Errorf("isPermanent(%v) = %v, want %v", err, got, want)
		}
	}
}
