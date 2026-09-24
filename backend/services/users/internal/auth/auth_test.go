package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", 4)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("hash must not equal the plaintext password")
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Error("VerifyPassword should accept the correct password")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Error("VerifyPassword should reject an incorrect password")
	}
}

func TestHashPasswordSaltsEachCallDifferently(t *testing.T) {
	h1, err := HashPassword("same-password", 4)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	h2, err := HashPassword("same-password", 4)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h1 == h2 {
		t.Error("hashing the same password twice should produce different hashes (per-call random salt)")
	}
	if !VerifyPassword(h1, "same-password") || !VerifyPassword(h2, "same-password") {
		t.Error("both independently-salted hashes should still verify the same password")
	}
}

func TestIssueTokenProducesAVerifiableJWT(t *testing.T) {
	secret := []byte("test-secret-at-least-32-bytes-long!!")
	userID := "d5a1f0c2-1111-4b2b-9a3e-0123456789ab"

	tokenStr, expiresAt, err := IssueToken(secret, userID, time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatal("expiresAt should be in the future")
	}

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("token is not a valid JWT signed with secret: %v", err)
	}
	if claims.Subject != userID {
		t.Errorf("token subject = %q, want %q", claims.Subject, userID)
	}

	if diff := claims.ExpiresAt.Time.Sub(expiresAt); diff > time.Second || diff < -time.Second {
		t.Errorf("token exp claim = %v, want ~%v", claims.ExpiresAt.Time, expiresAt)
	}

	if _, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte("a-completely-different-secret-32-bytes!"), nil
	}); err == nil {
		t.Error("token should not verify against a different secret")
	}
}
