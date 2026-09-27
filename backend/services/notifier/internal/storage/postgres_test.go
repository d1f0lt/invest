package storage

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	dsn := os.Getenv("NOTIFIER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTIFIER_TEST_DATABASE_URL not set")
	}
	s, err := NewStore(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	mustExec(t, s, `DELETE FROM alerts; DELETE FROM telegram_links; DELETE FROM telegram_link_tokens;
		DELETE FROM latest_prices WHERE secid LIKE 'T\_%'; DELETE FROM securities WHERE secid LIKE 'T\_%'`)
	mustExec(t, s, `INSERT INTO securities (secid, board, short_name, currency, decimals) VALUES
		('T_SBER','TQBR','Сбер','SUR',2), ('T_DUAL','TQBR',NULL,'SUR',2), ('T_DUAL','TQTF',NULL,'SUR',2)`)
	mustExec(t, s, `INSERT INTO securities (secid, board, short_name, currency, decimals, face_value, price_in_percent)
		VALUES ('T_OFZ','TQOB','ОФЗ','SUR',4,1000,true)`)
	return s
}

func mustExec(t *testing.T, s *Store, q string, args ...any) {
	t.Helper()
	if _, err := s.db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func setPrice(t *testing.T, s *Store, secid, board string, price float64, status string, age time.Duration) {
	mustExec(t, s, `INSERT INTO latest_prices (secid, board, last_price, trading_status, collected_at)
		VALUES ($1,$2,$3,$4, now() - make_interval(secs => $5))
		ON CONFLICT (secid, board) DO UPDATE SET last_price = EXCLUDED.last_price,
			trading_status = EXCLUDED.trading_status, collected_at = EXCLUDED.collected_at`,
		secid, board, price, status, age.Seconds())
}

const (
	u1 = "11111111-1111-1111-1111-111111111111"
	u2 = "22222222-2222-2222-2222-222222222222"
)

func TestInstrument(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	setPrice(t, s, "T_SBER", "TQBR", 300, "T", 0)

	in, err := s.Instrument(ctx, "T_SBER", "")
	if err != nil || in.Board != "TQBR" || in.LastPrice == nil || *in.LastPrice != 300 || *in.Decimals != 2 {
		t.Fatalf("got %+v %v", in, err)
	}
	if _, err := s.Instrument(ctx, "T_DUAL", ""); !errors.Is(err, ErrAmbiguousBoard) {
		t.Errorf("dual: %v", err)
	}
	if in, err := s.Instrument(ctx, "T_DUAL", "TQTF"); err != nil || in.LastPrice != nil || in.ShortName != "T_DUAL" {
		t.Errorf("dual TQTF: %+v %v", in, err)
	}
	if _, err := s.Instrument(ctx, "T_NONE", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("none: %v", err)
	}
}

func TestAlertsCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	setPrice(t, s, "T_SBER", "TQBR", 305.5, "T", 0)

	a, err := s.CreateAlert(ctx, Alert{UserID: u1, SecID: "T_SBER", Board: "TQBR", Direction: DirectionAbove, BasePrice: 300, TargetPrice: 310})
	if err != nil || a.ID == "" {
		t.Fatal(err)
	}
	if _, err := s.CreateAlert(ctx, Alert{UserID: u1, SecID: "T_NOPE", Board: "TQBR", Direction: DirectionAbove, BasePrice: 1, TargetPrice: 2}); err == nil {
		t.Error("FK on securities must reject unknown instruments")
	}
	list, err := s.ListAlerts(ctx, u1)
	if err != nil || len(list) != 1 || list[0].ShortName != "Сбер" || list[0].CurrentPrice == nil || *list[0].CurrentPrice != 305.5 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if n, _ := s.CountAlerts(ctx, u1); n != 1 {
		t.Errorf("count = %d", n)
	}
	if other, _ := s.ListAlerts(ctx, u2); len(other) != 0 {
		t.Errorf("u2 sees %v", other)
	}
	if err := s.DeleteAlert(ctx, u2, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign delete: %v", err)
	}
	if err := s.DeleteAlert(ctx, u1, "not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad id: %v", err)
	}
	a.SecID, a.Board, a.Direction, a.BasePrice, a.TargetPrice, a.InputPercent = "T_OFZ", "TQOB", DirectionBelow, 100, 95, &[]float64{-5}[0]
	if _, err := s.UpdateAlert(ctx, a); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := s.GetAlert(ctx, u1, a.ID)
	if err != nil || got.SecID != "T_OFZ" || got.ShortName != "ОФЗ" || !got.PriceInPercent || got.InputPercent == nil || *got.InputPercent != -5 {
		t.Fatalf("after update: %+v %v", got, err)
	}
	a.InputPercent = nil
	if _, err := s.UpdateAlert(ctx, a); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := s.GetAlert(ctx, u1, a.ID); got.InputPercent != nil {
		t.Errorf("input_percent must be cleared: %v", *got.InputPercent)
	}
	if err := s.DeleteAlert(ctx, u1, a.ID); err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestTriggeredAlerts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stale := 5 * time.Minute

	up, _ := s.CreateAlert(ctx, Alert{UserID: u1, SecID: "T_SBER", Board: "TQBR", Direction: DirectionAbove, BasePrice: 300, TargetPrice: 310})
	down, _ := s.CreateAlert(ctx, Alert{UserID: u1, SecID: "T_OFZ", Board: "TQOB", Direction: DirectionBelow, BasePrice: 60, TargetPrice: 58})
	_, _ = s.CreateAlert(ctx, Alert{UserID: u2, SecID: "T_SBER", Board: "TQBR", Direction: DirectionAbove, BasePrice: 300, TargetPrice: 310})

	ids := func() map[string]bool {
		t.Helper()
		got, err := s.TriggeredAlerts(ctx, stale)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, a := range got {
			m[a.ID] = true
			if a.ChatID != 100 {
				t.Errorf("chat = %d", a.ChatID)
			}
		}
		return m
	}

	setPrice(t, s, "T_SBER", "TQBR", 312, "T", 0)
	setPrice(t, s, "T_OFZ", "TQOB", 57.9, "T", 0)
	if got := ids(); len(got) != 0 {
		t.Errorf("no telegram link yet, got %v", got)
	}

	if err := s.CreateLinkToken(ctx, u1, "h1", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeLinkToken(ctx, "h1", 100, "andrey"); err != nil {
		t.Fatal(err)
	}
	if got := ids(); !got[up.ID] || !got[down.ID] || len(got) != 2 {
		t.Errorf("both of u1's alerts should trigger, got %v", got)
	}

	setPrice(t, s, "T_SBER", "TQBR", 309.99, "T", 0)
	if got := ids(); got[up.ID] {
		t.Errorf("below target must not trigger")
	}
	setPrice(t, s, "T_SBER", "TQBR", 310, "T", 0)
	if got := ids(); !got[up.ID] {
		t.Errorf("at target must trigger")
	}
	setPrice(t, s, "T_SBER", "TQBR", 320, "N", 0)
	if got := ids(); got[up.ID] {
		t.Errorf("non-trading status must not trigger")
	}
	setPrice(t, s, "T_SBER", "TQBR", 320, "T", 10*time.Minute)
	if got := ids(); got[up.ID] {
		t.Errorf("stale quote must not trigger")
	}
	setPrice(t, s, "T_SBER", "TQBR", 320, "T", 0)

	got, _ := s.TriggeredAlerts(ctx, stale)
	var sent TriggeredAlert
	for _, a := range got {
		if a.ID == up.ID {
			sent = a
		}
	}
	if err := s.MarkTriggered(ctx, up.ID, sent.UpdatedAt, 320); err != nil {
		t.Fatal(err)
	}
	if got := ids(); got[up.ID] || !got[down.ID] {
		t.Errorf("triggered alert must be skipped: %v", got)
	}
	a, err := s.GetAlert(ctx, u1, up.ID)
	if err != nil || a.TriggeredAt == nil || a.TriggeredPrice == nil || *a.TriggeredPrice != 320 || a.CurrentPrice == nil {
		t.Fatalf("get triggered: %+v %v", a, err)
	}
	if _, err := s.GetAlert(ctx, u2, up.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign get: %v", err)
	}

	a.Direction, a.BasePrice, a.TargetPrice = DirectionAbove, 320, 330
	upd, err := s.UpdateAlert(ctx, a)
	if err != nil || upd.TriggeredAt != nil || !upd.UpdatedAt.After(sent.UpdatedAt) {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if got := ids(); got[up.ID] {
		t.Errorf("320 < new target 330, must not trigger")
	}
	setPrice(t, s, "T_SBER", "TQBR", 331, "T", 0)
	if got := ids(); !got[up.ID] {
		t.Errorf("reactivated alert must trigger again")
	}

	if err := s.MarkTriggered(ctx, up.ID, sent.UpdatedAt, 331); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.GetAlert(ctx, u1, up.ID); a.TriggeredAt != nil {
		t.Errorf("stale mark must be ignored")
	}

	a.UserID = u2
	if _, err := s.UpdateAlert(ctx, a); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign update: %v", err)
	}
}

func TestTelegramLinking(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if _, err := s.GetTelegramLink(ctx, u1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found: %v", err)
	}
	_ = s.CreateLinkToken(ctx, u1, "old", time.Now().Add(-time.Minute))
	if _, err := s.ConsumeLinkToken(ctx, "old", 100, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired: %v", err)
	}
	_ = s.CreateLinkToken(ctx, u1, "a", time.Now().Add(time.Minute))
	_ = s.CreateLinkToken(ctx, u1, "b", time.Now().Add(time.Minute))
	if _, err := s.ConsumeLinkToken(ctx, "a", 100, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("superseded token: %v", err)
	}
	user, err := s.ConsumeLinkToken(ctx, "b", 100, "andrey")
	if err != nil || user != u1 {
		t.Fatalf("consume: %q %v", user, err)
	}
	if _, err := s.ConsumeLinkToken(ctx, "b", 100, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("token must be single-use: %v", err)
	}
	l, err := s.GetTelegramLink(ctx, u1)
	if err != nil || l.ChatID != 100 || l.Username != "andrey" {
		t.Fatalf("link: %+v %v", l, err)
	}

	_ = s.CreateLinkToken(ctx, u2, "c", time.Now().Add(time.Minute))
	if _, err := s.ConsumeLinkToken(ctx, "c", 100, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTelegramLink(ctx, u1); !errors.Is(err, ErrNotFound) {
		t.Errorf("u1 should lose the chat: %v", err)
	}
	_ = s.CreateLinkToken(ctx, u2, "d", time.Now().Add(time.Minute))
	if _, err := s.ConsumeLinkToken(ctx, "d", 200, ""); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.GetTelegramLink(ctx, u2); l.ChatID != 200 || l.Username != "" {
		t.Errorf("relink: %+v", l)
	}

	if ok, _ := s.DeleteTelegramLinkByChat(ctx, 200); !ok {
		t.Error("unlink by chat")
	}
	if ok, _ := s.DeleteTelegramLinkByChat(ctx, 200); ok {
		t.Error("second unlink must report false")
	}
	if err := s.DeleteTelegramLink(ctx, u2); err != nil {
		t.Error(err)
	}
}
