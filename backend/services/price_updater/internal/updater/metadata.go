package updater

import (
	"context"
	"log/slog"
	"time"

	"invest/backend/services/price_updater/internal/dohod"
	"invest/backend/services/price_updater/internal/moex"
	"invest/backend/services/price_updater/internal/storage"
)

type MetadataClient interface {
	Description(context.Context, string) ([]moex.DescriptionField, error)
	EmitterTitle(context.Context, string) (string, error)
}

type DividendClient interface {
	Dividends(context.Context, string) ([]dohod.Dividend, error)
}

type MetadataStore interface {
	MetadataTargets(context.Context, []string) ([]storage.MetadataTarget, error)
	SaveSecurityMetadata(context.Context, string, map[string]string, string) error
	SaveDividends(context.Context, string, []dohod.Dividend) error
}

type MetadataJob struct {
	Client       MetadataClient
	Dividends    DividendClient
	Store        MetadataStore
	Boards       []string
	Log          *slog.Logger
	RequestPause time.Duration
}

func (j *MetadataJob) Run(ctx context.Context) {
	for {
		j.RunOnce(ctx)
		timer := time.NewTimer(15 * time.Minute)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (j *MetadataJob) RunOnce(ctx context.Context) {
	targets, err := j.Store.MetadataTargets(ctx, j.Boards)
	if err != nil {
		j.Log.Error("metadata targets failed", "error", err)
		return
	}
	for i, target := range targets {
		if ctx.Err() != nil {
			return
		}
		if i > 0 && j.RequestPause > 0 {
			timer := time.NewTimer(j.RequestPause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if target.InfoDue {
			if err := j.syncInfo(ctx, target.SecID); err != nil {
				j.Log.Warn("security metadata sync failed", "secid", target.SecID, "error", err)
			}
		}
		if target.DividendsDue {
			dividends, err := j.Dividends.Dividends(ctx, target.SecID)
			if err == nil {
				err = j.Store.SaveDividends(ctx, target.SecID, dividends)
			}
			if err != nil {
				j.Log.Warn("dividends sync failed", "secid", target.SecID, "error", err)
			}
		}
	}
}

func (j *MetadataJob) syncInfo(ctx context.Context, secid string) error {
	description, err := j.Client.Description(ctx, secid)
	if err != nil {
		return err
	}
	issuer, err := j.Client.EmitterTitle(ctx, secid)
	if err != nil {
		return err
	}
	values := make(map[string]string, len(description))
	for _, field := range description {
		if field.Value != "" {
			values[field.Name] = field.Value
		}
	}
	return j.Store.SaveSecurityMetadata(ctx, secid, values, issuer)
}
