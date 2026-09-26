package pnl

import (
	"math"
	"testing"
	"time"
)

var msk = time.FixedZone("MSK", 3*60*60)

func mskDay(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, msk) }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestValueHistory_TradesClosesAndCash(t *testing.T) {
	trades := []Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 100, Fee: 1, ExecutedAt: mskDay(2026, 9, 2).Add(11 * time.Hour)},
		{SecID: "SBER", Board: "TQBR", Side: Sell, Quantity: 4, Price: 120, ExecutedAt: mskDay(2026, 9, 5).Add(12 * time.Hour)},
	}
	cash := []CashFlow{
		{Type: CashDeposit, Amount: 2000, OccurredAt: mskDay(2026, 9, 1).Add(10 * time.Hour)},
		{Type: CashDividend, Amount: 30, SecID: "SBER", Board: "TQBR", OccurredAt: mskDay(2026, 9, 4).Add(9 * time.Hour)},
	}
	closes := map[string][]DailyClose{
		"SBER/TQBR": {
			{Day: mskDay(2026, 9, 2), Close: 105},
			{Day: mskDay(2026, 9, 3), Close: 110},
			{Day: mskDay(2026, 9, 5), Close: 118},
		},
	}
	days := HistoryDays(mskDay(2026, 9, 1), mskDay(2026, 9, 6).Add(15*time.Hour), msk, 400)
	now := mskDay(2026, 9, 6).Add(15 * time.Hour)
	got := ValueHistory(trades, cash, closes, map[string]float64{"SBER/TQBR": 125}, days, now)

	want := []float64{
		2000,
		2000 - 1001 + 10*105,
		999 + 10*110,
		999 + 30 + 10*110,
		1029 + 480 + 6*118,
		1509 + 6*125,
	}
	if len(got) != len(want) {
		t.Fatalf("points = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !near(got[i].Value, want[i]) {
			t.Errorf("day %d: value = %v, want %v", i+1, got[i].Value, want[i])
		}
		if !near(got[i].NetDeposits, 2000) {
			t.Errorf("day %d: net deposits = %v", i+1, got[i].NetDeposits)
		}
		if !got[i].Day.Equal(mskDay(2026, 9, 1+i)) {
			t.Errorf("day %d: date = %v", i+1, got[i].Day)
		}
	}
}

func TestValueHistory_FallbackToTradePriceAndRedemption(t *testing.T) {
	trades := []Trade{
		{SecID: "BOND", Board: "TQCB", Side: Buy, Quantity: 2, Price: 990, ExecutedAt: mskDay(2026, 9, 1).Add(time.Hour)},
		{SecID: "FUND", Board: "TQTF", Side: Buy, Quantity: 5, Price: 10, ExecutedAt: mskDay(2026, 9, 1).Add(time.Hour)},
	}
	cash := []CashFlow{
		{Type: CashDeposit, Amount: 2050, OccurredAt: mskDay(2026, 9, 1)},
		{Type: CashRedemption, Amount: 2000, SecID: "BOND", Board: "TQCB", OccurredAt: mskDay(2026, 9, 3).Add(10 * time.Hour)},
	}
	closes := map[string][]DailyClose{"BOND/TQCB": {{Day: mskDay(2026, 9, 2), Close: 995}}}
	days := HistoryDays(mskDay(2026, 9, 1), mskDay(2026, 9, 3), msk, 400)
	got := ValueHistory(trades, cash, closes, nil, days, mskDay(2026, 9, 10))

	want := []float64{
		20 + 2*990 + 5*10,
		20 + 2*995 + 5*10,
		2020 + 5*10,
	}
	for i := range want {
		if !near(got[i].Value, want[i]) {
			t.Errorf("day %d: value = %v, want %v", i+1, got[i].Value, want[i])
		}
	}
}

func TestHistoryDays_ThinsToWeeksKeepingLast(t *testing.T) {
	from := mskDay(2025, 1, 1)
	to := mskDay(2026, 9, 6).Add(20 * time.Hour)
	days := HistoryDays(from, to, msk, 400)
	if len(days) > 400/7+100 {
		t.Fatalf("not thinned: %d days", len(days))
	}
	if !days[len(days)-1].Equal(mskDay(2026, 9, 6)) {
		t.Errorf("last = %v", days[len(days)-1])
	}
	for i := 1; i < len(days); i++ {
		if days[i].Sub(days[i-1]) != 7*24*time.Hour {
			t.Fatalf("step %v at %d", days[i].Sub(days[i-1]), i)
		}
	}
	if short := HistoryDays(mskDay(2026, 9, 1), mskDay(2026, 9, 1), msk, 400); len(short) != 1 {
		t.Errorf("single day = %v", short)
	}
}

func TestApplyDayChange(t *testing.T) {
	trades := []Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 100, ExecutedAt: mskDay(2026, 9, 1)},
		{SecID: "GAZP", Board: "TQBR", Side: Buy, Quantity: 3, Price: 150, ExecutedAt: mskDay(2026, 9, 1)},
	}
	s := Compute(trades, nil, map[string]float64{"SBER/TQBR": 110, "GAZP/TQBR": 140})
	ApplyDayChange(&s, map[string]float64{"SBER/TQBR": 105})
	if !near(s.TotalDayChange, 50) {
		t.Errorf("total day change = %v, want 50", s.TotalDayChange)
	}
	for _, in := range s.Instruments {
		switch in.SecID {
		case "SBER":
			if in.DayChange == nil || !near(*in.DayChange, 50) {
				t.Errorf("SBER day change = %v", in.DayChange)
			}
		case "GAZP":
			if in.DayChange != nil {
				t.Errorf("GAZP without prev close: %v", *in.DayChange)
			}
		}
	}
}
