package authmd

import (
	"context"

	"google.golang.org/grpc/metadata"
)

const UserIDKey = "x-user-id"

func WithUserID(ctx context.Context, userID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, UserIDKey, userID)
}
