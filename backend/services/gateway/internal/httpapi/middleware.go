package httpapi

import (
	"context"
	"net/http"
	"strings"

	"invest/backend/services/gateway/internal/auth"
)

type contextKey int

const userIDContextKey contextKey = 0

func (h *Handlers) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, prefix) {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		userID, err := auth.VerifyToken(h.JWTSecret, strings.TrimSpace(authz[len(prefix):]))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func userIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDContextKey).(string)
	return id, ok
}
