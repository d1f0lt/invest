package config

import "testing"

func TestLoadRejectsNonPositiveTokenTTLs(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	for _, key := range []string{"ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL"} {
		for _, v := range []string{"0s", "-5m"} {
			t.Run(key+"="+v, func(t *testing.T) {
				t.Setenv(key, v)
				if _, err := Load(); err == nil {
					t.Errorf("expected error")
				}
			})
		}
	}
	if _, err := Load(); err != nil {
		t.Errorf("defaults: unexpected error %v", err)
	}
}
