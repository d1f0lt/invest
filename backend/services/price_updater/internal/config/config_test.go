package config

import "testing"

func TestLoadRejectsNonPositivePollInterval(t *testing.T) {
	for _, v := range []string{"0s", "-1m"} {
		t.Setenv("POLL_INTERVAL", v)
		if _, err := Load(); err == nil {
			t.Errorf("POLL_INTERVAL=%s: expected error", v)
		}
	}
	t.Setenv("POLL_INTERVAL", "30s")
	if _, err := Load(); err != nil {
		t.Errorf("POLL_INTERVAL=30s: unexpected error %v", err)
	}
}
