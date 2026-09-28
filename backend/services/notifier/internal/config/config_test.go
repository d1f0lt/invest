package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GRPCAddr != ":8085" || cfg.CheckInterval.String() != "1m0s" || cfg.MaxAlertsPerUser != 100 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.TelegramBotToken != "" {
		t.Errorf("bot token must default to empty")
	}
}

func TestLoadRejectsNonPositive(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"CHECK_INTERVAL", "0s"},
		{"CHECK_INTERVAL", "-1m"},
		{"PRICE_STALE_AFTER", "0"},
		{"LINK_TOKEN_TTL", "-5m"},
		{"MAX_ALERTS_PER_USER", "0"},
		{"MAX_ALERTS_PER_USER", "abc"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Errorf("expected error")
			}
		})
	}
}
