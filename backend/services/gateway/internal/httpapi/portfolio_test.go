package httpapi

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
)

func TestToHoldingResponse_OpenPositionCarriesPriceFields(t *testing.T) {
	price, mv, upnl := 120.0, 1200.0, 200.0
	h := &portfoliopb.Holding{
		Secid: "SBER", Board: "TQBR", Quantity: 10, AvgCost: 100,
		CurrentPrice: &price, MarketValue: &mv, UnrealizedPnl: &upnl,
	}

	out := toHoldingResponse(h)
	if out.CurrentPrice == nil || *out.CurrentPrice != 120 {
		t.Errorf("CurrentPrice = %v, want 120", out.CurrentPrice)
	}
	if out.MarketValue == nil || *out.MarketValue != 1200 {
		t.Errorf("MarketValue = %v, want 1200", out.MarketValue)
	}
	if out.UnrealizedPnL == nil || *out.UnrealizedPnL != 200 {
		t.Errorf("UnrealizedPnL = %v, want 200", out.UnrealizedPnL)
	}
}

func TestToHoldingResponse_NoPriceDataOmitsFields(t *testing.T) {
	h := &portfoliopb.Holding{Secid: "SBER", Board: "TQBR", Quantity: 10, AvgCost: 100}

	out := toHoldingResponse(h)
	if out.CurrentPrice != nil || out.MarketValue != nil || out.UnrealizedPnL != nil {
		t.Errorf("expected all price fields nil, got %+v", out)
	}
}

func TestToInstrumentResponse_EmbedsHoldingAndAddsTotals(t *testing.T) {
	i := &portfoliopb.InstrumentPnL{
		Holding:     &portfoliopb.Holding{Secid: "GAZP", Board: "TQBR", Quantity: 5, AvgCost: 50},
		RealizedPnl: 30,
		TotalPnl:    30,
	}

	out := toInstrumentResponse(i)
	if out.SecID != "GAZP" || out.RealizedPnL != 30 || out.TotalPnL != 30 {
		t.Errorf("unexpected instrument response: %+v", out)
	}
}

func TestToTradeResponse_RoundTripsAllFields(t *testing.T) {
	executedAt := time.Now().UTC().Truncate(time.Second)
	tr := &portfoliopb.Trade{
		Id: "t1", Secid: "SBER", Board: "TQBR", Side: "buy",
		Quantity: 10, Price: 250.5, Fee: 5, Currency: "RUB",
		ExecutedAt: timestamppb.New(executedAt),
	}

	out := toTradeResponse(tr)
	if out.ID != "t1" || out.SecID != "SBER" || out.Side != "buy" || out.Currency != "RUB" {
		t.Errorf("unexpected trade response: %+v", out)
	}
	if !out.ExecutedAt.Equal(executedAt) {
		t.Errorf("ExecutedAt = %v, want %v", out.ExecutedAt, executedAt)
	}
}
