package linktoken

import (
	"regexp"
	"testing"
)

func TestGenerate(t *testing.T) {
	tok, hash, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(tok) {
		t.Errorf("token %q is not a valid Telegram start parameter", tok)
	}
	if hash != Hash(tok) || len(hash) != 64 {
		t.Errorf("hash mismatch")
	}
	tok2, _, _ := Generate()
	if tok == tok2 {
		t.Errorf("tokens must be random")
	}
}
