package httpapi

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	securitiesreaderpb "invest/backend/services/gateway/internal/securitiesreaderpb"
)

func TestToPriceView_CarriesOptionalFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	last := 289.5
	prev := 291.25
	p := &securitiesreaderpb.PriceView{
		Secid: "SBER", Board: "TQBR",
		LastPrice:   &last,
		PrevClose:   &prev,
		CollectedAt: timestamppb.New(now),
	}

	out := toPriceView(p)
	if out.SecID != "SBER" || out.Board != "TQBR" {
		t.Errorf("unexpected identity fields: %+v", out)
	}
	if out.Last == nil || *out.Last != 289.5 {
		t.Errorf("Last = %v, want 289.5", out.Last)
	}
	if out.PrevClose == nil || *out.PrevClose != 291.25 {
		t.Errorf("PrevClose = %v, want 291.25", out.PrevClose)
	}
	if out.ShortName != nil {
		t.Errorf("ShortName = %v, want nil (never sent by price_updater for this instrument)", out.ShortName)
	}
	if !out.CollectedAt.Equal(now) {
		t.Errorf("CollectedAt = %v, want %v", out.CollectedAt, now)
	}
}
