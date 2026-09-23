// Package authmd attaches caller identity to outgoing gRPC calls made to
// the portfolio service. This is the client-side counterpart of the
// "x-user-id" metadata convention used throughout this project's
// internal gRPC APIs (see e.g.
// backend/services/portfolio/internal/authmd, which reads this same
// key on the server side) - the direct replacement for the old
// X-User-ID HTTP header this service's portfolioclient used to set. See
// architecture-decisions.md, "перевод внутреннего взаимодействия
// сервисов на gRPC".
package authmd

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// UserIDKey must match the key every backend gRPC service reads the
// caller's identity from.
const UserIDKey = "x-user-id"

// WithUserID returns a context whose outgoing gRPC metadata carries
// userID, for use as the ctx argument to any portfolio-service RPC call
// made on that user's behalf.
func WithUserID(ctx context.Context, userID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, UserIDKey, userID)
}
