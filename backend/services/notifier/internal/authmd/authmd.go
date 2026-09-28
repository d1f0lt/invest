package authmd

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const UserIDKey = "x-user-id"

func UserID(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if values := md.Get(UserIDKey); len(values) > 0 && values[0] != "" {
			return values[0], nil
		}
	}
	return "", status.Error(codes.Unauthenticated, "missing "+UserIDKey+" metadata")
}
