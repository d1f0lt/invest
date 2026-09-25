package cleanup

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu      sync.Mutex
	calls   int
	cutoffs []time.Time
	deleted int64
	err     error
}

func (f *fakeStore) DeleteExpiredRefreshTokens(_ context.Context, cutoff time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.cutoffs = append(f.cutoffs, cutoff)
	return f.deleted, f.err
}

func newTestRunner(store Store, interval, retention time.Duration) *Runner {
	return &Runner{
		Store:     store,
		Interval:  interval,
		Retention: retention,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestRun_DeletesImmediatelyAndOnTick(t *testing.T) {
	store := &fakeStore{deleted: 3}
	r := newTestRunner(store, 10*time.Millisecond, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Millisecond)
	defer cancel()
	r.Run(ctx) 

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.calls < 2 {
		t.Errorf("delete calls = %d, want at least 2 (immediate + ticks)", store.calls)
	}
	for _, cutoff := range store.cutoffs {
		if want := time.Now().Add(-time.Hour); cutoff.After(want) {
			t.Errorf("cutoff = %v, want about an hour in the past (retention)", cutoff)
		}
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	store := &fakeStore{}
	r := newTestRunner(store, time.Hour, 0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.calls != 1 {
		t.Errorf("delete calls = %d, want exactly 1 (the immediate one; interval never fires)", store.calls)
	}
}

func TestRun_SurvivesStoreError(t *testing.T) {
	store := &fakeStore{err: context.DeadlineExceeded}
	r := newTestRunner(store, 5*time.Millisecond, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	r.Run(ctx) 

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.calls < 2 {
		t.Errorf("delete calls = %d, want at least 2 (runner must keep going after an error)", store.calls)
	}
}
