package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func IssueToken(secret []byte, userID string, ttl time.Duration) (token string, expiresAt time.Time, err error) {
	now := time.Now()
	expiresAt = now.Add(ttl)

	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}
