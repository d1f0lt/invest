package grpcserver

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"invest/backend/services/notifier/internal/linktoken"
	"invest/backend/services/notifier/internal/storage"
	notifierpb "invest/backend/services/notifier/proto"
)

type fakeStore struct {
	instruments map[string][]storage.Instrument
	alerts      []storage.Alert
	count       int
	tokenUser   string
	tokenHash   string
	tokenExp    time.Time
	link        *storage.TelegramLink
}

func (f *fakeStore) Instrument(_ context.Context, secid, board string) (storage.Instrument, error) {
	var found []storage.Instrument
	for _, in := range f.instruments[secid] {
		if board == "" || in.Board == board {
			found = append(found, in)
		}
	}
	switch len(found) {
	case 0:
		return storage.Instrument{}, storage.ErrNotFound
	case 1:
		return found[0], nil
	}
	return storage.Instrument{}, storage.ErrAmbiguousBoard
}

func (f *fakeStore) CountAlerts(context.Context, string) (int, error) { return f.count, nil }

func (f *fakeStore) CreateAlert(_ context.Context, a storage.Alert) (storage.Alert, error) {
	a.ID = "a1"
	a.CreatedAt = time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	f.alerts = append(f.alerts, a)
	return a, nil
}

func (f *fakeStore) GetAlert(_ context.Context, userID, id string) (storage.Alert, error) {
	for _, a := range f.alerts {
		if a.ID == id && a.UserID == userID {
			return a, nil
		}
	}
	return storage.Alert{}, storage.ErrNotFound
}

func (f *fakeStore) UpdateAlert(_ context.Context, a storage.Alert) (storage.Alert, error) {
	for i := range f.alerts {
		if f.alerts[i].ID == a.ID && f.alerts[i].UserID == a.UserID {
			a.TriggeredAt, a.TriggeredPrice = nil, nil
			a.UpdatedAt = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
			f.alerts[i] = a
			return a, nil
		}
	}
	return storage.Alert{}, storage.ErrNotFound
}

func (f *fakeStore) ListAlerts(context.Context, string) ([]storage.Alert, error) {
	return f.alerts, nil
}

func (f *fakeStore) DeleteAlert(_ context.Context, userID, id string) error {
	if id != "a1" || userID != "u1" {
		return storage.ErrNotFound
	}
	return nil
}

func (f *fakeStore) CreateLinkToken(_ context.Context, userID, hash string, exp time.Time) error {
	f.tokenUser, f.tokenHash, f.tokenExp = userID, hash, exp
	return nil
}

func (f *fakeStore) GetTelegramLink(context.Context, string) (storage.TelegramLink, error) {
	if f.link == nil {
		return storage.TelegramLink{}, storage.ErrNotFound
	}
	return *f.link, nil
}

func (f *fakeStore) DeleteTelegramLink(context.Context, string) error { f.link = nil; return nil }

type fakeBot struct{ name string }

func (b fakeBot) Username() (string, bool) { return b.name, b.name != "" }

func ptr[T any](v T) *T { return &v }

func newServer() (*Server, *fakeStore) {
	st := &fakeStore{instruments: map[string][]storage.Instrument{
		"SBER": {{SecID: "SBER", Board: "TQBR", ShortName: "Сбербанк", Currency: "SUR", Decimals: ptr(2), LastPrice: ptr(300.0)}},
		"DUAL": {{SecID: "DUAL", Board: "TQBR", LastPrice: ptr(10.0)}, {SecID: "DUAL", Board: "TQTF", LastPrice: ptr(10.0)}},
		"NOPX": {{SecID: "NOPX", Board: "TQBR"}},
	}}
	return &Server{
		Store: st, Bot: fakeBot{"invest_alerts_bot"}, LinkTokenTTL: 15 * time.Minute,
		MaxAlertsPerUser: 100, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now: func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) },
	}, st
}

func userCtx() context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "u1"))
}

func code(err error) codes.Code { return status.Code(err) }

func TestCreateAlert_Percent(t *testing.T) {
	s, st := newServer()
	a, err := s.CreateAlert(userCtx(), &notifierpb.CreateAlertRequest{
		Secid: "sber", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: 3.333},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.TargetPrice != 310 || a.BasePrice != 300 || a.Direction != notifierpb.Direction_DIRECTION_ABOVE {
		t.Errorf("unexpected alert: %+v", a)
	}
	if a.ChangePercent != 3.33 || a.Board != "TQBR" || a.ShortName != "Сбербанк" {
		t.Errorf("unexpected alert: %+v", a)
	}
	if st.alerts[0].UserID != "u1" || st.alerts[0].TargetPrice != 310 {
		t.Errorf("stored: %+v", st.alerts[0])
	}
}

func TestCreateAlert_TargetBelowIsDown(t *testing.T) {
	s, _ := newServer()
	a, err := s.CreateAlert(userCtx(), &notifierpb.CreateAlertRequest{
		Secid: "SBER", Board: "tqbr", Target: &notifierpb.CreateAlertRequest_TargetPrice{TargetPrice: 250},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Direction != notifierpb.Direction_DIRECTION_BELOW || a.TargetPrice != 250 {
		t.Errorf("unexpected alert: %+v", a)
	}
}

func TestCreateAlert_Errors(t *testing.T) {
	cases := []struct {
		name string
		req  *notifierpb.CreateAlertRequest
		want codes.Code
	}{
		{"no secid", &notifierpb.CreateAlertRequest{Target: &notifierpb.CreateAlertRequest_TargetPrice{TargetPrice: 1}}, codes.InvalidArgument},
		{"no target", &notifierpb.CreateAlertRequest{Secid: "SBER"}, codes.InvalidArgument},
		{"zero price", &notifierpb.CreateAlertRequest{Secid: "SBER", Target: &notifierpb.CreateAlertRequest_TargetPrice{TargetPrice: 0}}, codes.InvalidArgument},
		{"negative price", &notifierpb.CreateAlertRequest{Secid: "SBER", Target: &notifierpb.CreateAlertRequest_TargetPrice{TargetPrice: -5}}, codes.InvalidArgument},
		{"equal to current", &notifierpb.CreateAlertRequest{Secid: "SBER", Target: &notifierpb.CreateAlertRequest_TargetPrice{TargetPrice: 300}}, codes.InvalidArgument},
		{"zero percent", &notifierpb.CreateAlertRequest{Secid: "SBER", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: 0}}, codes.InvalidArgument},
		{"-100 percent", &notifierpb.CreateAlertRequest{Secid: "SBER", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: -100}}, codes.InvalidArgument},
		{"huge percent", &notifierpb.CreateAlertRequest{Secid: "SBER", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: 5000}}, codes.InvalidArgument},
		{"unknown", &notifierpb.CreateAlertRequest{Secid: "NOPE", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: 5}}, codes.NotFound},
		{"ambiguous board", &notifierpb.CreateAlertRequest{Secid: "DUAL", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: 5}}, codes.InvalidArgument},
		{"no price", &notifierpb.CreateAlertRequest{Secid: "NOPX", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: 5}}, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newServer()
			if _, err := s.CreateAlert(userCtx(), tc.req); code(err) != tc.want {
				t.Errorf("code = %v, want %v (%v)", code(err), tc.want, err)
			}
		})
	}

	s, _ := newServer()
	if _, err := s.CreateAlert(context.Background(), cases[0].req); code(err) != codes.Unauthenticated {
		t.Errorf("no user: %v", err)
	}
}

func TestCreateAlert_Limit(t *testing.T) {
	s, st := newServer()
	st.count = 100
	_, err := s.CreateAlert(userCtx(), &notifierpb.CreateAlertRequest{
		Secid: "SBER", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: 5},
	})
	if code(err) != codes.FailedPrecondition {
		t.Errorf("code = %v", code(err))
	}
}

func TestDeleteAlert(t *testing.T) {
	s, _ := newServer()
	if _, err := s.DeleteAlert(userCtx(), &notifierpb.DeleteAlertRequest{Id: "a1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteAlert(userCtx(), &notifierpb.DeleteAlertRequest{Id: "other"}); code(err) != codes.NotFound {
		t.Errorf("code = %v", code(err))
	}
}

func TestCreateTelegramLink(t *testing.T) {
	s, st := newServer()
	resp, err := s.CreateTelegramLink(userCtx(), &emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	prefix := "https://t.me/invest_alerts_bot?start="
	if !strings.HasPrefix(resp.Url, prefix) {
		t.Fatalf("url = %q", resp.Url)
	}
	token := strings.TrimPrefix(resp.Url, prefix)
	if linktoken.Hash(token) != st.tokenHash || st.tokenUser != "u1" {
		t.Errorf("stored hash/user mismatch")
	}
	if !resp.ExpiresAt.AsTime().Equal(s.Now().Add(15 * time.Minute)) {
		t.Errorf("expires_at = %v", resp.ExpiresAt.AsTime())
	}
}

func TestCreateTelegramLink_BotStates(t *testing.T) {
	s, _ := newServer()
	s.Bot = nil
	if _, err := s.CreateTelegramLink(userCtx(), &emptypb.Empty{}); code(err) != codes.FailedPrecondition {
		t.Errorf("no bot: %v", err)
	}
	s.Bot = fakeBot{}
	if _, err := s.CreateTelegramLink(userCtx(), &emptypb.Empty{}); code(err) != codes.FailedPrecondition {
		t.Errorf("bot not ready: %v", err)
	}
}

func TestGetTelegramLink(t *testing.T) {
	s, st := newServer()
	resp, err := s.GetTelegramLink(userCtx(), &emptypb.Empty{})
	if err != nil || resp.Linked || !resp.BotEnabled {
		t.Fatalf("unlinked: %+v %v", resp, err)
	}
	st.link = &storage.TelegramLink{UserID: "u1", ChatID: 42, Username: "andrey", LinkedAt: time.Now()}
	resp, err = s.GetTelegramLink(userCtx(), &emptypb.Empty{})
	if err != nil || !resp.Linked || resp.Username != "andrey" || resp.LinkedAt == nil {
		t.Fatalf("linked: %+v %v", resp, err)
	}
}

func TestUpdateAlert_ReactivatesWithNewBase(t *testing.T) {
	s, st := newServer()
	trigAt := time.Date(2026, 9, 24, 11, 0, 0, 0, time.UTC)
	st.alerts = []storage.Alert{{
		ID: "a1", UserID: "u1", SecID: "SBER", Board: "TQBR", Direction: storage.DirectionAbove,
		BasePrice: 280, TargetPrice: 295, TriggeredAt: &trigAt, TriggeredPrice: ptr(296.0),
	}}
	list, _ := s.ListAlerts(userCtx(), &emptypb.Empty{})
	if a := list.Alerts[0]; a.Status != notifierpb.AlertStatus_ALERT_STATUS_TRIGGERED || a.TriggeredAt == nil || a.GetTriggeredPrice() != 296 {
		t.Fatalf("expected triggered: %+v", a)
	}

	a, err := s.UpdateAlert(userCtx(), &notifierpb.UpdateAlertRequest{
		Id: "a1", Target: &notifierpb.UpdateAlertRequest_ChangePercent{ChangePercent: -10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != notifierpb.AlertStatus_ALERT_STATUS_ACTIVE || a.TriggeredAt != nil || a.TriggeredPrice != nil {
		t.Errorf("must be active again: %+v", a)
	}
	if a.BasePrice != 300 || a.TargetPrice != 270 || a.Direction != notifierpb.Direction_DIRECTION_BELOW || a.ShortName != "Сбербанк" {
		t.Errorf("unexpected: %+v", a)
	}
	if st.alerts[0].TargetPrice != 270 || st.alerts[0].TriggeredAt != nil {
		t.Errorf("stored: %+v", st.alerts[0])
	}
}

func TestUpdateAlert_Errors(t *testing.T) {
	s, st := newServer()
	st.alerts = []storage.Alert{{ID: "a1", UserID: "u1", SecID: "SBER", Board: "TQBR"}}
	for name, tc := range map[string]struct {
		req  *notifierpb.UpdateAlertRequest
		want codes.Code
	}{
		"no target": {&notifierpb.UpdateAlertRequest{Id: "a1"}, codes.InvalidArgument},
		"unknown":   {&notifierpb.UpdateAlertRequest{Id: "nope", Target: &notifierpb.UpdateAlertRequest_TargetPrice{TargetPrice: 310}}, codes.NotFound},
		"equal":     {&notifierpb.UpdateAlertRequest{Id: "a1", Target: &notifierpb.UpdateAlertRequest_TargetPrice{TargetPrice: 300}}, codes.InvalidArgument},
	} {
		if _, err := s.UpdateAlert(userCtx(), tc.req); code(err) != tc.want {
			t.Errorf("%s: code = %v, want %v", name, code(err), tc.want)
		}
	}
	other := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-user-id", "u2"))
	if _, err := s.UpdateAlert(other, &notifierpb.UpdateAlertRequest{Id: "a1", Target: &notifierpb.UpdateAlertRequest_TargetPrice{TargetPrice: 310}}); code(err) != codes.NotFound {
		t.Errorf("foreign alert: %v", err)
	}
}

func TestUpdateAlert_ChangesSecurityAndKeepsInput(t *testing.T) {
	s, st := newServer()
	st.instruments["GAZP"] = []storage.Instrument{{SecID: "GAZP", Board: "TQBR", ShortName: "Газпром", Decimals: ptr(2), LastPrice: ptr(120.0)}}
	st.alerts = []storage.Alert{{ID: "a1", UserID: "u1", SecID: "SBER", Board: "TQBR", Direction: storage.DirectionAbove, BasePrice: 300, TargetPrice: 330, InputPercent: ptr(10.0)}}

	a, err := s.UpdateAlert(userCtx(), &notifierpb.UpdateAlertRequest{
		Id: "a1", Secid: "gazp", Target: &notifierpb.UpdateAlertRequest_TargetPrice{TargetPrice: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Secid != "GAZP" || a.Board != "TQBR" || a.BasePrice != 120 || a.Direction != notifierpb.Direction_DIRECTION_BELOW || a.InputPercent != nil {
		t.Errorf("unexpected: %+v", a)
	}
	if st.alerts[0].SecID != "GAZP" || st.alerts[0].InputPercent != nil {
		t.Errorf("stored: %+v", st.alerts[0])
	}

	a, err = s.UpdateAlert(userCtx(), &notifierpb.UpdateAlertRequest{
		Id: "a1", Target: &notifierpb.UpdateAlertRequest_ChangePercent{ChangePercent: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Secid != "GAZP" || a.TargetPrice != 126 || a.GetInputPercent() != 5 {
		t.Errorf("unexpected: %+v", a)
	}

	if _, err := s.UpdateAlert(userCtx(), &notifierpb.UpdateAlertRequest{
		Id: "a1", Secid: "DUAL", Target: &notifierpb.UpdateAlertRequest_ChangePercent{ChangePercent: 5},
	}); code(err) != codes.InvalidArgument {
		t.Errorf("ambiguous board: %v", err)
	}
}

func TestCreateAlert_InputPercent(t *testing.T) {
	s, _ := newServer()
	a, err := s.CreateAlert(userCtx(), &notifierpb.CreateAlertRequest{
		Secid: "SBER", Target: &notifierpb.CreateAlertRequest_ChangePercent{ChangePercent: -3.5},
	})
	if err != nil || a.GetInputPercent() != -3.5 {
		t.Fatalf("%+v %v", a, err)
	}
	a, err = s.CreateAlert(userCtx(), &notifierpb.CreateAlertRequest{
		Secid: "SBER", Target: &notifierpb.CreateAlertRequest_TargetPrice{TargetPrice: 320},
	})
	if err != nil || a.InputPercent != nil {
		t.Fatalf("%+v %v", a, err)
	}
}
