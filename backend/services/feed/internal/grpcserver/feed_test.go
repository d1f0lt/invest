package grpcserver

import (
	"testing"
	"time"

	feedpb "invest/backend/services/feed/proto"
)

func TestForecastRecommendationAndMoscowDeadline(t *testing.T) {
	now := time.Date(2026, 11, 21, 21, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name           string
		change         float64
		recommendation string
		date           string
		valid          bool
	}{
		{"growth and sale", 5, "sell", "2026-11-23", false},
		{"decline and purchase", -5, "buy", "2026-11-23", false},
		{"growth and purchase", 5, "buy", "2026-11-23", true},
		{"decline and sale", -5, "sell", "2026-11-23", true},
		{"decline and hold", -5, "hold", "2026-11-23", true},
		{"growth and hold", 5, "hold", "2026-11-23", true},
		{"today in Moscow", 5, "hold", "2026-11-22", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft := &feedpb.CreatePostRequest{
				Kind: "forecast", Title: "Title", Body: "Body", Tickers: []string{"SBER"},
				Board: "TQBR", ChangePercent: &tc.change, Recommendation: tc.recommendation,
				TargetDate: tc.date,
			}
			if message := validateDraft(draft, now.In(feedTimezone).AddDate(0, 0, 1)); (message == "") != tc.valid {
				t.Fatalf("valid = %v, validation message = %q", tc.valid, message)
			}
		})
	}
}
