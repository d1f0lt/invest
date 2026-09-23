package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"invest/backend/services/price_updater/internal/config"
	"invest/backend/services/price_updater/internal/moexclient"
	"invest/backend/services/price_updater/internal/storage"
	"invest/backend/services/price_updater/internal/updater"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := storage.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	client := moexclient.New(cfg.MoexISSBaseURL, cfg.HTTPTimeout)

	
	
	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Error("failed to load Europe/Moscow time zone", "error", err)
		os.Exit(1)
	}

	log.Info("price_updater starting",
		"boards", cfg.MoexBoards,
		"poll_interval", cfg.PollInterval.String(),
		"history_run_at", time.Date(0, 1, 1, cfg.HistoryRunHour, cfg.HistoryRunMinute, 0, 0, moscow).Format("15:04")+" MSK",
		"backfill_days", cfg.BackfillDays,
	)

	u := updater.New(client, store, cfg.MoexBoards, moscow, log)
	h := updater.NewHistoryJob(client, store, cfg.MoexBoards, moscow, updater.HistoryConfig{
		RunAtHour:    cfg.HistoryRunHour,
		RunAtMinute:  cfg.HistoryRunMinute,
		BackfillDays: cfg.BackfillDays,
		RequestPause: cfg.HistoryRequestPause,
	}, log)

	
	
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); u.Run(ctx, cfg.PollInterval) }()
	go func() { defer wg.Done(); h.Run(ctx) }()
	wg.Wait()

	log.Info("price_updater stopped")
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
