package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-at-least-32-bytes-long!"

func signTestToken(t *testing.T, secret []byte, sub string, ttl time.Duration) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   sub,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
	}).SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func TestRequireAuth_PutsVerifiedUserIDInContext(t *testing.T) {
	secret := []byte(testSecret)
	h := &Handlers{JWTSecret: secret}

	var gotUserID string
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, gotOK = userIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+signTestToken(t, secret, "real-user", time.Hour))
	rec := httptest.NewRecorder()

	h.requireAuth(next.ServeHTTP).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !gotOK || gotUserID != "real-user" {
		t.Errorf("userIDFromContext = (%q, %v), want (%q, true)", gotUserID, gotOK, "real-user")
	}
}

func TestRequireAuth_RejectsMissingToken(t *testing.T) {
	h := &Handlers{JWTSecret: []byte(testSecret)}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()
	h.requireAuth(next.ServeHTTP).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("next should not have been called")
	}
}

func TestRequireAuth_RejectsInvalidToken(t *testing.T) {
	h := &Handlers{JWTSecret: []byte(testSecret)}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	h.requireAuth(next.ServeHTTP).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("next should not have been called")
	}
}
