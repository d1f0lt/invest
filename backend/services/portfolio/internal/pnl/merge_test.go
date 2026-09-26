package pnl

import (
	"testing"
	"time"
)

func TestMerge_SingleSummaryUnchanged(t *testing.T) {
	s := Compute([]Trade{{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 100}}, nil,
		map[string]float64{"SBER/TQBR": 110})
	got := Merge([]Summary{s})
	if len(got.Instruments) != 1 || got.TotalUnrealizedPnL != s.TotalUnrealizedPnL {
		t.Fatalf("got %+v, want %+v", got, s)
	}
}

func TestMerge_SumsAccountsSeparately(t *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := map[string]float64{"SBER/TQBR": 300, "GAZP/TQBR": 150}

	a := Compute([]Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 10, Price: 200, ExecutedAt: day},
		{SecID: "SBER", Board: "TQBR", Side: Sell, Quantity: 5, Price: 250, ExecutedAt: day.Add(time.Hour)},
	}, []CashFlow{
		{Type: CashDeposit, Amount: 5000, OccurredAt: day},
		{Type: CashDividend, Amount: 30, SecID: "SBER", Board: "TQBR", OccurredAt: day},
	}, prices)
	b := Compute([]Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 5, Price: 260, ExecutedAt: day},
		{SecID: "GAZP", Board: "TQBR", Side: Buy, Quantity: 2, Price: 100, ExecutedAt: day},
	}, []CashFlow{
		{Type: CashDeposit, Amount: 2000, OccurredAt: day},
		{Type: CashTax, Amount: -4, OccurredAt: day},
	}, prices)
	ApplyDayChange(&a, map[string]float64{"SBER/TQBR": 290})
	ApplyDayChange(&b, map[string]float64{"SBER/TQBR": 290})

	got := Merge([]Summary{a, b})

	if len(got.Instruments) != 2 || got.Instruments[0].SecID != "GAZP" || got.Instruments[1].SecID != "SBER" {
		t.Fatalf("instruments = %+v", got.Instruments)
	}
	sber := got.Instruments[1]
	if !near(sber.Quantity, 10) || !near(sber.AvgCost, 230) {
		t.Fatalf("SBER qty/avg = %v/%v, want 10/230", sber.Quantity, sber.AvgCost)
	}
	if !near(sber.RealizedPnL, 250) || !near(*sber.UnrealizedPnL, 700) || !near(*sber.MarketValue, 3000) {
		t.Fatalf("SBER = %+v", sber)
	}
	if !near(sber.Dividends, 30) || !near(*sber.DayChange, 100) || !near(*sber.CurrentPrice, 300) {
		t.Fatalf("SBER = %+v", sber)
	}

	if !near(got.NetDeposits, 7000) || !near(got.TotalTaxes, -4) || !near(got.TotalDividends, 30) {
		t.Fatalf("totals = %+v", got)
	}
	if !near(got.CashBalance, a.CashBalance+b.CashBalance) || !near(got.TotalDayChange, 100) {
		t.Fatalf("cash/day = %v/%v", got.CashBalance, got.TotalDayChange)
	}
	if !near(got.TotalPnL(), a.TotalPnL()+b.TotalPnL()) {
		t.Fatalf("total pnl = %v, want %v", got.TotalPnL(), a.TotalPnL()+b.TotalPnL())
	}
}

func TestMerge_ClosedPositionHasNoPrice(t *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := map[string]float64{"SBER/TQBR": 300}
	a := Compute([]Trade{
		{SecID: "SBER", Board: "TQBR", Side: Buy, Quantity: 1, Price: 200, ExecutedAt: day},
		{SecID: "SBER", Board: "TQBR", Side: Sell, Quantity: 1, Price: 250, ExecutedAt: day.Add(time.Hour)},
	}, nil, prices)
	b := Compute(nil, []CashFlow{{Type: CashDeposit, Amount: 100, OccurredAt: day}}, prices)

	got := Merge([]Summary{a, b})
	if len(got.Instruments) != 1 || got.Instruments[0].Quantity != 0 || got.Instruments[0].MarketValue != nil {
		t.Fatalf("instruments = %+v", got.Instruments)
	}
	if len(Holdings(got)) != 0 {
		t.Fatalf("holdings = %+v", Holdings(got))
	}
}

func TestMergeValueHistory(t *testing.T) {
	d1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := d1.AddDate(0, 0, 1)
	got := MergeValueHistory([][]ValuePoint{
		{{Day: d1, Value: 100, NetDeposits: 90}, {Day: d2, Value: 110, NetDeposits: 90}},
		{{Day: d1, Value: 0, NetDeposits: 0}, {Day: d2, Value: 50, NetDeposits: 40}},
	})
	if len(got) != 2 || got[0].Value != 100 || got[1].Value != 160 || got[1].NetDeposits != 130 || !got[1].Day.Equal(d2) {
		t.Fatalf("got %+v", got)
	}
	if MergeValueHistory(nil) != nil {
		t.Fatal("want nil for no histories")
	}
}
