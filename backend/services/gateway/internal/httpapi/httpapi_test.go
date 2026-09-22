package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWriteUpstreamError_MapsGRPCCodesToHTTPStatus(t *testing.T) {
	cases := []struct {
		code codes.Code
		want int
	}{
		{codes.InvalidArgument, 400},
		{codes.Unauthenticated, 401},
		{codes.NotFound, 404},
		{codes.AlreadyExists, 409},
		{codes.FailedPrecondition, 422},
		{codes.Unavailable, 502},
		{codes.Internal, 500},
		{codes.Unknown, 500},
	}

	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeUpstreamError(rec, testLogger(), status.Error(c.code, "boom"))
		if rec.Code != c.want {
			t.Errorf("code %v: status = %d, want %d", c.code, rec.Code, c.want)
		}
	}
}

func TestWriteUpstreamError_NonGRPCErrorIsBadGateway(t *testing.T) {
	rec := httptest.NewRecorder()
	writeUpstreamError(rec, testLogger(), errors.New("connection refused"))
	if rec.Code != 502 {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}
