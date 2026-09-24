package pnl

import (
	"math"
	"testing"
	"time"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func t0(hours int) time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(hours) * time.Hour)
}

func TestComputeInstrumentFullyClosedPositionStillReportsRealizedPnL(t *testing.T) {
	trades := []Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 100, ExecutedAt: t0(0)},
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 120, ExecutedAt: t0(1)},

		{SecID: "SBER", Board: "TQBR", Side: Sell, Quantity: 15, Price: 130, ExecutedAt: t0(2)},

		{SecID: "SBER", Board: "TQBR", Side: Sell, Quantity: 5, Price: 140, ExecutedAt: t0(3)},
	}

	summary := Compute(trades, nil, nil)
	if len(summary.Instruments) != 1 {
		t.Fatalf("got %d instruments, want 1", len(summary.Instruments))
	}
	inst := summary.Instruments[0]

	if !almostEqual(inst.RealizedPnL, 450) {
		t.Errorf("RealizedPnL = %v, want 450", inst.RealizedPnL)
	}
	if !almostEqual(inst.Quantity, 0) {
		t.Errorf("Quantity = %v, want 0 (fully closed)", inst.Quantity)
	}
	if inst.UnrealizedPnL != nil {
		t.Errorf("UnrealizedPnL = %v, want nil for a closed position", *inst.UnrealizedPnL)
	}
	if !almostEqual(inst.TotalPnL(), 450) {
		t.Errorf("TotalPnL() = %v, want 450", inst.TotalPnL())
	}

	if got := Holdings(summary); len(got) != 0 {
		t.Errorf("Holdings() = %v, want none open", got)
	}
}

func TestComputeInstrumentOpenPositionUsesCurrentPrice(t *testing.T) {
	trades := []Trade{
		{SecID: "GAZP", Board: "TQBR", Side: Buy, Quantity: 100, Price: 150, ExecutedAt: t0(0)},
		{SecID: "GAZP", Board: "TQBR", Side: Buy, Quantity: 50, Price: 180, ExecutedAt: t0(1)},
	}

	prices := map[string]float64{PriceKey("GAZP", "TQBR"): 200}
	summary := Compute(trades, nil, prices)
	inst := summary.Instruments[0]

	if !almostEqual(inst.Quantity, 150) {
		t.Errorf("Quantity = %v, want 150", inst.Quantity)
	}
	if !almostEqual(inst.AvgCost, 160) {
		t.Errorf("AvgCost = %v, want 160", inst.AvgCost)
	}
	if inst.UnrealizedPnL == nil {
		t.Fatal("UnrealizedPnL should be set when a current price is supplied")
	}

	if !almostEqual(*inst.UnrealizedPnL, 6000) {
		t.Errorf("UnrealizedPnL = %v, want 6000", *inst.UnrealizedPnL)
	}
	if !almostEqual(summary.TotalUnrealizedPnL, 6000) {
		t.Errorf("TotalUnrealizedPnL = %v, want 6000", summary.TotalUnrealizedPnL)
	}

	holdings := Holdings(summary)
	if len(holdings) != 1 || holdings[0].SecID != "GAZP" {
		t.Errorf("Holdings() = %+v, want just GAZP", holdings)
	}
}

func TestComputeInstrumentMissingPriceLeavesUnrealizedNil(t *testing.T) {
	trades := []Trade{
		{SecID: "LKOH", Board: "TQBR", Side: Buy, Quantity: 1, Price: 7000, ExecutedAt: t0(0)},
	}
	summary := Compute(trades, nil, map[string]float64{})
	inst := summary.Instruments[0]
	if inst.CurrentPrice != nil || inst.UnrealizedPnL != nil || inst.MarketValue != nil {
		t.Errorf("expected nil price/value/pnl fields without a known price, got %+v", inst)
	}
}

func TestComputeFeesReduceRealizedPnL(t *testing.T) {
	trades := []Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 100, Fee: 5, ExecutedAt: t0(0)},

		{SecID: "SBER", Board: "TQBR", Side: Sell, Quantity: 10, Price: 110, Fee: 2, ExecutedAt: t0(1)},
	}
	summary := Compute(trades, nil, nil)
	if !almostEqual(summary.Instruments[0].RealizedPnL, 93) {
		t.Errorf("RealizedPnL = %v, want 93", summary.Instruments[0].RealizedPnL)
	}
}

func TestComputeIgnoresTradeOrderInSliceButUsesExecutedAt(t *testing.T) {
	trades := []Trade{
		{SecID: "SBER", Board: "TQBR", Side: Sell, Quantity: 5, Price: 130, ExecutedAt: t0(2)},
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 5, Price: 100, ExecutedAt: t0(0)},
	}
	summary := Compute(trades, nil, nil)
	inst := summary.Instruments[0]
	if !almostEqual(inst.RealizedPnL, 150) {
		t.Errorf("RealizedPnL = %v, want 150", inst.RealizedPnL)
	}
}

func TestIncomeTaxesAndDepositsInSummary(t *testing.T) {
	trades := []Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 300, Fee: 1, ExecutedAt: t0(0)},
	}
	cash := []CashFlow{
		{Type: CashDeposit, Amount: 5000, OccurredAt: t0(-1)},
		{Type: CashWithdrawal, Amount: -1000, OccurredAt: t0(5)},
		{Type: CashDividend, Amount: 300, SecID: "SBER", Board: "TQBR", OccurredAt: t0(3)},
		{Type: CashTax, Amount: -50, OccurredAt: t0(4)},
		{Type: CashFee, Amount: -10, OccurredAt: t0(4)},
	}
	s := Compute(trades, cash, map[string]float64{PriceKey("SBER", "TQBR"): 320})

	if !almostEqual(s.NetDeposits, 4000) {
		t.Errorf("NetDeposits = %v, want 4000", s.NetDeposits)
	}
	// 5000 - 1000 - (3000 + 1) + 300 - 50 - 10
	if !almostEqual(s.CashBalance, 1239) {
		t.Errorf("CashBalance = %v, want 1239", s.CashBalance)
	}
	// unrealized 10*320 - 3001 = 199; + dividend 300 - tax 50 - fee 10
	if !almostEqual(s.TotalPnL(), 439) {
		t.Errorf("TotalPnL = %v, want 439", s.TotalPnL())
	}
	// Market value + cash - net deposits must equal total P&L.
	mv := *s.Instruments[0].MarketValue
	if !almostEqual(mv+s.CashBalance-s.NetDeposits, s.TotalPnL()) {
		t.Errorf("value identity broken: mv %v + cash %v - deposits %v != pnl %v", mv, s.CashBalance, s.NetDeposits, s.TotalPnL())
	}
	inst := s.Instruments[0]
	if !almostEqual(inst.Dividends, 300) || !almostEqual(inst.TotalPnL(), 499) {
		t.Errorf("instrument dividends %v total %v, want 300, 499", inst.Dividends, inst.TotalPnL())
	}
}

func TestBondAccruedInterestCouponAndRedemption(t *testing.T) {
	// 3 bonds at 841 RUB, 82.26 НКД paid; coupon 112.20; then redeemed at
	// 1000 each.
	trades := []Trade{
		{SecID: "SU26254RMFS1", Board: "TQOB", Side: Buy, Quantity: 3, Price: 841, Fee: 7.83, AccruedInterest: 82.26, ExecutedAt: t0(0)},
	}
	cash := []CashFlow{
		{Type: CashCoupon, Amount: 112.20, SecID: "SU26254RMFS1", Board: "TQOB", OccurredAt: t0(10)},
		{Type: CashRedemption, Amount: 3000, SecID: "SU26254RMFS1", Board: "TQOB", OccurredAt: t0(20)},
	}
	s := Compute(trades, cash, nil)
	inst := s.Instruments[0]

	if !almostEqual(inst.AccruedInterest, -82.26) || !almostEqual(inst.Coupons, 112.20) {
		t.Errorf("accrued %v coupons %v", inst.AccruedInterest, inst.Coupons)
	}
	// cost 2523 + 7.83 = 2530.83, redeemed 3000 -> realized 469.17
	if !almostEqual(inst.RealizedPnL, 469.17) {
		t.Errorf("RealizedPnL = %v, want 469.17", inst.RealizedPnL)
	}
	// 469.17 + 112.20 - 82.26
	if !almostEqual(s.TotalPnL(), 499.11) {
		t.Errorf("TotalPnL = %v, want 499.11", s.TotalPnL())
	}
}

func TestDividendWithoutTradesStillCounted(t *testing.T) {
	s := Compute(nil, []CashFlow{{Type: CashDividend, Amount: 10, SecID: "GAZP", Board: "TQBR"}}, nil)
	if len(s.Instruments) != 1 || !almostEqual(s.Instruments[0].Dividends, 10) || !almostEqual(s.TotalPnL(), 10) {
		t.Errorf("summary = %+v", s)
	}
	if len(Holdings(s)) != 0 {
		t.Error("an instrument with only dividends is not a holding")
	}
}
