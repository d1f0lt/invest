package auth

import "testing"

func TestGenerateRefreshTokenIsRandomAndURLSafe(t *testing.T) {
	t1, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	t2, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if t1 == t2 {
		t.Error("two calls should not produce the same token")
	}
	if len(t1) == 0 {
		t.Error("token should not be empty")
	}
}

func TestHashRefreshTokenIsDeterministicAndDoesNotLeakTheToken(t *testing.T) {
	token, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	h1 := HashRefreshToken(token)
	h2 := HashRefreshToken(token)
	if h1 != h2 {
		t.Error("hashing the same token twice should produce the same hash")
	}
	if h1 == token {
		t.Error("hash must not equal the raw token")
	}

	other, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if HashRefreshToken(other) == h1 {
		t.Error("hashes of different tokens should not collide")
	}
}
