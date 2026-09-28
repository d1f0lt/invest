package checker

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"invest/backend/services/notifier/internal/storage"
	"invest/backend/services/notifier/internal/telegram"
)

type fakeStore struct {
	triggered []storage.TriggeredAlert
	marked    []string
	unlinked  []int64
}

func (f *fakeStore) TriggeredAlerts(_ context.Context, _ time.Duration) ([]storage.TriggeredAlert, error) {
	return f.triggered, nil
}

func (f *fakeStore) MarkTriggered(_ context.Context, id string, updatedAt time.Time, price float64) error {
	if !updatedAt.Equal(time.Date(2026, 9, 20, 21, 30, 0, 0, time.UTC)) || price != 312.4 {
		panic("MarkTriggered must get the sent version and price")
	}
	f.marked = append(f.marked, id)
	return nil
}

func (f *fakeStore) DeleteTelegramLinkByChat(_ context.Context, chatID int64) (bool, error) {
	f.unlinked = append(f.unlinked, chatID)
	return true, nil
}

type fakeSender struct {
	sent []int64
	errs map[int64]error
}

func (f *fakeSender) SendMessage(_ context.Context, chatID int64, _ string) error {
	if err := f.errs[chatID]; err != nil {
		return err
	}
	f.sent = append(f.sent, chatID)
	return nil
}

func alert(id string, chat int64) storage.TriggeredAlert {
	return storage.TriggeredAlert{
		Alert: storage.Alert{ID: id, SecID: "SBER", ShortName: "Сбербанк", Currency: "SUR",
			Direction: storage.DirectionAbove, BasePrice: 300, TargetPrice: 310,
			CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 9, 20, 21, 30, 0, 0, time.UTC)},
		ChatID: chat, LastPrice: 312.4,
	}
}

func newChecker(st *fakeStore, snd *fakeSender) *Checker {
	loc, _ := time.LoadLocation("Europe/Moscow")
	return &Checker{
		Store: st, Sender: snd, Loc: loc, StaleAfter: 5 * time.Minute,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestRunOnce_SendsAndMarks(t *testing.T) {
	st := &fakeStore{triggered: []storage.TriggeredAlert{alert("a1", 1), alert("a2", 2)}}
	snd := &fakeSender{}
	if err := newChecker(st, snd).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(snd.sent) != 2 || len(st.marked) != 2 {
		t.Errorf("sent %v, marked %v", snd.sent, st.marked)
	}
}

func TestRunOnce_BlockedChatIsUnlinkedAndNotMarked(t *testing.T) {
	st := &fakeStore{triggered: []storage.TriggeredAlert{alert("a1", 1), alert("a2", 1), alert("a3", 2)}}
	snd := &fakeSender{errs: map[int64]error{1: &telegram.APIError{Code: 403, Description: "Forbidden: bot was blocked by the user"}}}
	if err := newChecker(st, snd).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.unlinked) != 1 || st.unlinked[0] != 1 {
		t.Errorf("unlinked %v", st.unlinked)
	}
	if len(st.marked) != 1 || st.marked[0] != "a3" {
		t.Errorf("marked %v", st.marked)
	}
}

func TestRunOnce_TransientErrorLeavesAlertForRetry(t *testing.T) {
	st := &fakeStore{triggered: []storage.TriggeredAlert{alert("a1", 1), alert("a2", 2)}}
	snd := &fakeSender{errs: map[int64]error{1: &telegram.APIError{Code: 500, Description: "oops"}}}
	_ = newChecker(st, snd).RunOnce(context.Background())
	if len(st.marked) != 1 || st.marked[0] != "a2" || len(st.unlinked) != 0 {
		t.Errorf("marked %v unlinked %v", st.marked, st.unlinked)
	}
}

func TestRunOnce_RateLimitStopsRound(t *testing.T) {
	st := &fakeStore{triggered: []storage.TriggeredAlert{alert("a1", 1), alert("a2", 2)}}
	snd := &fakeSender{errs: map[int64]error{1: &telegram.APIError{Code: 429, RetryAfter: 3 * time.Second}}}
	_ = newChecker(st, snd).RunOnce(context.Background())
	if len(snd.sent) != 0 || len(st.marked) != 0 {
		t.Errorf("sent %v marked %v", snd.sent, st.marked)
	}
}

func TestFormatMessage(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	msg := FormatMessage(alert("a1", 1), loc)
	for _, want := range []string{"▲ SBER (Сбербанк): 312,4 ₽", "выросла до цели 310 ₽", "(300 ₽, 21.09.2026)", "+4,13%", "обновите его в приложении"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}

	bond := alert("b1", 1)
	bond.SecID, bond.ShortName, bond.PriceInPercent = "SU26238RMFS4", "ОФЗ 26238", true
	bond.Direction, bond.BasePrice, bond.TargetPrice, bond.LastPrice = storage.DirectionBelow, 60, 58, 57.95
	msg = FormatMessage(bond, loc)
	for _, want := range []string{"▼", "57,95% номинала", "снизилась до цели 58% номинала", "-3,42%"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
}
