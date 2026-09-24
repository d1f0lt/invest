package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-at-least-32-bytes-long!"

func sign(t *testing.T, secret []byte, claims jwt.RegisteredClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func TestVerifyToken_Valid(t *testing.T) {
	secret := []byte(testSecret)
	now := time.Now()
	tok := sign(t, secret, jwt.RegisteredClaims{
		Subject:   "user-123",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	})

	userID, err := VerifyToken(secret, tok)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if userID != "user-123" {
		t.Errorf("userID = %q, want %q", userID, "user-123")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	secret := []byte(testSecret)
	now := time.Now()
	tok := sign(t, secret, jwt.RegisteredClaims{
		Subject:   "user-123",
		IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
	})

	if _, err := VerifyToken(secret, tok); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	secret := []byte(testSecret)
	other := []byte("a-completely-different-secret-32b!!")
	tok := sign(t, secret, jwt.RegisteredClaims{
		Subject:   "user-123",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	if _, err := VerifyToken(other, tok); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyToken_NoSubject(t *testing.T) {
	secret := []byte(testSecret)
	tok := sign(t, secret, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	if _, err := VerifyToken(secret, tok); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyToken_Malformed(t *testing.T) {
	secret := []byte(testSecret)
	if _, err := VerifyToken(secret, "not-a-jwt"); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyToken_WrongAlgorithm(t *testing.T) {
	secret := []byte(testSecret)
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject:   "user-123",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none-alg token: %v", err)
	}

	if _, err := VerifyToken(secret, tok); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}
