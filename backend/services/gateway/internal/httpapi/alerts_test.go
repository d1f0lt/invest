package httpapi

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	notifierpb "invest/backend/services/gateway/internal/notifierpb"
)

func f64(v float64) *float64 { return &v }

func TestToCreateAlertPB(t *testing.T) {
	pb, problem := toCreateAlertPB(createAlertRequest{SecID: "SBER", TargetPrice: f64(310)})
	if problem != "" || pb.GetTargetPrice() != 310 || pb.GetSecid() != "SBER" {
		t.Errorf("target price: %v %q", pb, problem)
	}
	pb, problem = toCreateAlertPB(createAlertRequest{SecID: "SBER", Board: "TQBR", ChangePercent: f64(-5)})
	if problem != "" || pb.GetChangePercent() != -5 || pb.GetBoard() != "TQBR" {
		t.Errorf("percent: %v %q", pb, problem)
	}
	for name, req := range map[string]createAlertRequest{
		"none":     {SecID: "SBER"},
		"both":     {SecID: "SBER", TargetPrice: f64(1), ChangePercent: f64(1)},
		"no secid": {TargetPrice: f64(1)},
	} {
		if _, problem := toCreateAlertPB(req); problem == "" {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestToAlertResponse(t *testing.T) {
	created := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	cur := 305.0
	out := toAlertResponse(&notifierpb.Alert{
		Id: "a1", Secid: "SBER", Board: "TQBR", Direction: notifierpb.Direction_DIRECTION_BELOW,
		BasePrice: 300, TargetPrice: 285, ChangePercent: -5, CurrentPrice: &cur,
		CreatedAt: timestamppb.New(created),
	})
	if out.Direction != "below" || out.CurrentPrice == nil || *out.CurrentPrice != 305 || !out.CreatedAt.Equal(created) || out.TriggeredAt != nil {
		t.Errorf("unexpected: %+v", out)
	}

	trig := 284.5
	out = toAlertResponse(&notifierpb.Alert{
		Id: "a1", Status: notifierpb.AlertStatus_ALERT_STATUS_TRIGGERED,
		TriggeredAt: timestamppb.New(created.Add(time.Hour)), TriggeredPrice: &trig,
		CreatedAt: timestamppb.New(created), UpdatedAt: timestamppb.New(created),
	})
	if out.InputPercent != nil {
		t.Errorf("input_percent must be null: %+v", out)
	}
	if out.Status != "triggered" || out.TriggeredAt == nil || out.TriggeredPrice == nil || *out.TriggeredPrice != 284.5 {
		t.Errorf("triggered: %+v", out)
	}
	if out := toAlertResponse(&notifierpb.Alert{Status: notifierpb.AlertStatus_ALERT_STATUS_ACTIVE, InputPercent: f64(5)}); out.Status != "active" || out.InputPercent == nil || *out.InputPercent != 5 {
		t.Errorf("active: %+v", out)
	}
}

func TestToUpdateAlertPB(t *testing.T) {
	pb, problem := toUpdateAlertPB("a1", updateAlertRequest{ChangePercent: f64(7)})
	if problem != "" || pb.GetId() != "a1" || pb.GetChangePercent() != 7 {
		t.Errorf("percent: %v %q", pb, problem)
	}
	pb, problem = toUpdateAlertPB("a1", updateAlertRequest{TargetPrice: f64(250)})
	if problem != "" || pb.GetTargetPrice() != 250 {
		t.Errorf("price: %v %q", pb, problem)
	}
	pb, problem = toUpdateAlertPB("a1", updateAlertRequest{SecID: "GAZP", Board: "TQBR", TargetPrice: f64(120)})
	if problem != "" || pb.GetSecid() != "GAZP" || pb.GetBoard() != "TQBR" {
		t.Errorf("security: %v %q", pb, problem)
	}
	if _, problem := toUpdateAlertPB("a1", updateAlertRequest{}); problem == "" {
		t.Error("empty body must be rejected")
	}
	if _, problem := toUpdateAlertPB("a1", updateAlertRequest{TargetPrice: f64(1), ChangePercent: f64(1)}); problem == "" {
		t.Error("both must be rejected")
	}
}

func TestToTelegramLinkResponse(t *testing.T) {
	out := toTelegramLinkResponse(&notifierpb.TelegramLink{BotEnabled: true})
	if out.Linked || out.LinkedAt != nil || !out.BotEnabled {
		t.Errorf("unlinked: %+v", out)
	}
	out = toTelegramLinkResponse(&notifierpb.TelegramLink{Linked: true, Username: "andrey", LinkedAt: timestamppb.Now(), BotEnabled: true})
	if !out.Linked || out.LinkedAt == nil || out.Username != "andrey" {
		t.Errorf("linked: %+v", out)
	}
}
