package cleanup

import (
	"context"
	"log/slog"
	"time"
)

type Store interface {
	DeleteExpiredRefreshTokens(ctx context.Context, cutoff time.Time) (int64, error)
}

type Runner struct {
	Store     Store
	Interval  time.Duration
	Retention time.Duration
	Log       *slog.Logger
}

func (r *Runner) Run(ctx context.Context) {
	r.deleteOnce(ctx)

	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.Log.Info("refresh token cleanup stopped")
			return
		case <-ticker.C:
			r.deleteOnce(ctx)
		}
	}
}

func (r *Runner) deleteOnce(ctx context.Context) {
	n, err := r.Store.DeleteExpiredRefreshTokens(ctx, time.Now().Add(-r.Retention))
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		r.Log.Error("delete expired refresh tokens", "error", err)
		return
	}
	if n > 0 {
		r.Log.Info("deleted expired refresh tokens", "count", n)
	}
}
