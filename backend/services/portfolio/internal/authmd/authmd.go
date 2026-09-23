// Package authmd extracts the caller's identity from gRPC request
// metadata. This is the direct replacement for the old X-User-ID HTTP
// header trust model (see architecture-decisions.md, "убрали
// JWT-проверку из всех бэкенд-сервисов" and "перевод внутреннего
// взаимодействия сервисов на gRPC"): gateway verifies the caller's JWT
// once, then sets the "x-user-id" metadata key on every gRPC call it
// makes on that caller's behalf. This package - like the HTTP header it
// replaces - trusts that value completely, with no cryptographic check
// of its own. That's safe only because this service is unreachable
// except from inside the docker-compose network, which is assumed to
// have no untrusted containers (see root README.md).
package authmd

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UserIDKey is the metadata key gateway sets. Lowercase because gRPC
// metadata keys are normalized to lowercase on the wire regardless of
// how they're set.
const UserIDKey = "x-user-id"

// ErrMissing is returned (wrapped in a gRPC Unauthenticated status) when
// a call that requires a caller identity doesn't carry one.
var ErrMissing = errors.New("missing " + UserIDKey + " metadata")

// UserID extracts the caller's user id from ctx's incoming gRPC
// metadata. Every RPC that used to require the X-User-ID HTTP header
// (i.e. everything that went through requireAuth) calls this first and
// returns its error directly.
func UserID(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if values := md.Get(UserIDKey); len(values) > 0 && values[0] != "" {
			return values[0], nil
		}
	}
	return "", status.Error(codes.Unauthenticated, ErrMissing.Error())
}
