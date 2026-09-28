package checker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"invest/backend/services/notifier/internal/storage"
	"invest/backend/services/notifier/internal/telegram"
)

type Store interface {
	TriggeredAlerts(ctx context.Context, staleAfter time.Duration) ([]storage.TriggeredAlert, error)
	MarkTriggered(ctx context.Context, id string, updatedAt time.Time, price float64) error
	DeleteTelegramLinkByChat(ctx context.Context, chatID int64) (bool, error)
}

type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type Checker struct {
	Store      Store
	Sender     Sender
	Loc        *time.Location
	Interval   time.Duration
	StaleAfter time.Duration
	Log        *slog.Logger

	SendPause time.Duration
}

func (c *Checker) Run(ctx context.Context) {
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for {
		if err := c.RunOnce(ctx); err != nil && ctx.Err() == nil {
			c.Log.Error("alert check failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (c *Checker) RunOnce(ctx context.Context) error {
	alerts, err := c.Store.TriggeredAlerts(ctx, c.StaleAfter)
	if err != nil {
		return err
	}

	blocked := map[int64]bool{}
	for i, a := range alerts {
		if blocked[a.ChatID] {
			continue
		}
		if i > 0 && c.SendPause > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.SendPause):
			}
		}

		err := c.Sender.SendMessage(ctx, a.ChatID, FormatMessage(a, c.Loc))
		if err != nil {
			if telegram.IsBlocked(err) {
				blocked[a.ChatID] = true
				if _, delErr := c.Store.DeleteTelegramLinkByChat(ctx, a.ChatID); delErr != nil {
					c.Log.Error("unlink blocked chat", "error", delErr)
				} else {
					c.Log.Info("telegram chat unreachable, unlinked", "user_id", a.UserID, "error", err)
				}
				continue
			}
			var apiErr *telegram.APIError
			if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
				c.Log.Warn("telegram rate limit, postponing the rest to the next round", "retry_after", apiErr.RetryAfter)
				return nil
			}
			c.Log.Warn("send alert failed", "alert_id", a.ID, "error", err)
			continue
		}
		if err := c.Store.MarkTriggered(ctx, a.ID, a.UpdatedAt, a.LastPrice); err != nil {
			c.Log.Error("mark alert triggered", "alert_id", a.ID, "error", err)
		}
	}
	return nil
}

func FormatMessage(a storage.TriggeredAlert, loc *time.Location) string {
	unit := priceUnit(a.Currency, a.PriceInPercent)
	arrow, verb := "▲", "выросла"
	if a.Direction == storage.DirectionBelow {
		arrow, verb = "▼", "снизилась"
	}

	title := a.SecID
	if a.ShortName != "" && a.ShortName != a.SecID {
		title = fmt.Sprintf("%s (%s)", a.SecID, a.ShortName)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s: %s%s\n", arrow, title, FormatPrice(a.LastPrice), unit)
	fmt.Fprintf(&sb, "Цена %s до цели %s%s.\n", verb, FormatPrice(a.TargetPrice), unit)
	fmt.Fprintf(&sb, "С момента настройки уведомления (%s%s, %s): %s\n\n",
		FormatPrice(a.BasePrice), unit, a.UpdatedAt.In(loc).Format("02.01.2006"),
		FormatPercent(ChangePercent(a.BasePrice, a.LastPrice)))
	sb.WriteString("Уведомление выполнено. Чтобы снова следить за ценой, обновите его в приложении.")
	return sb.String()
}

func ChangePercent(base, price float64) float64 {
	if base == 0 {
		return 0
	}
	return (price - base) / base * 100
}

func FormatPrice(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1)
}

func FormatPercent(p float64) string {
	p = math.Round(p*100) / 100
	if p == 0 {
		p = 0
	}
	return strings.Replace(fmt.Sprintf("%+.2f%%", p), ".", ",", 1)
}

func priceUnit(currency string, inPercent bool) string {
	if inPercent {
		return "% номинала"
	}
	switch strings.ToUpper(currency) {
	case "SUR", "RUB", "":
		return " ₽"
	case "USD":
		return " $"
	case "EUR":
		return " €"
	case "CNY":
		return " ¥"
	default:
		return " " + currency
	}
}
