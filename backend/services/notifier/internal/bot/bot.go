package bot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"invest/backend/services/notifier/internal/linktoken"
	"invest/backend/services/notifier/internal/storage"
	"invest/backend/services/notifier/internal/telegram"
)

const pollTimeout = 50 * time.Second

type API interface {
	GetMe(ctx context.Context) (telegram.User, error)
	GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]telegram.Update, error)
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type Store interface {
	ConsumeLinkToken(ctx context.Context, tokenHash string, chatID int64, username string) (string, error)
	DeleteTelegramLinkByChat(ctx context.Context, chatID int64) (bool, error)
}

type Bot struct {
	API   API
	Store Store
	Log   *slog.Logger

	mu       sync.RWMutex
	username string
}

func (b *Bot) Username() (string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.username, b.username != ""
}

func (b *Bot) Run(ctx context.Context) {
	if !b.resolveUsername(ctx) {
		return
	}

	var offset int64
	backoff := time.Second
	for ctx.Err() == nil {
		updates, err := b.API.GetUpdates(ctx, offset, pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.Log.Warn("telegram getUpdates failed", "error", err, "retry_in", backoff)
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message != nil {
				b.HandleMessage(ctx, u.Message)
			}
		}
	}
}

func (b *Bot) resolveUsername(ctx context.Context) bool {
	backoff := 5 * time.Second
	for {
		me, err := b.API.GetMe(ctx)
		if err == nil && me.Username != "" {
			b.mu.Lock()
			b.username = me.Username
			b.mu.Unlock()
			b.Log.Info("telegram bot ready", "username", me.Username)
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		b.Log.Warn("telegram getMe failed", "error", err, "retry_in", backoff)
		if !sleep(ctx, backoff) {
			return false
		}
		backoff = min(backoff*2, 5*time.Minute)
	}
}

const (
	msgLinked = "Готово, Telegram подключён. Сюда будут приходить уведомления о ценах, " +
		"которые вы настроите в приложении.\n\nОтключить: /stop"
	msgBadToken = "Ссылка недействительна или устарела. Откройте в приложении вкладку " +
		"«Уведомления» и нажмите «Подключить Telegram» ещё раз."
	msgHello = "Это бот уведомлений о ценах Invest. Чтобы подключить его, откройте в приложении " +
		"вкладку «Уведомления» и нажмите «Подключить Telegram»."
	msgUnlinked    = "Telegram отключён, уведомления больше не придут. Подключить снова можно из приложения."
	msgNotLinked   = "Этот чат не подключён ни к одному аккаунту."
	msgHelp        = "Я только присылаю уведомления о ценах. Настраиваются они в приложении, на вкладке «Уведомления».\n\nОтключить: /stop"
	msgInternalErr = "Что-то пошло не так, попробуйте ещё раз чуть позже."
)

func (b *Bot) HandleMessage(ctx context.Context, m *telegram.Message) {
	if m.Chat.Type != "private" {
		return
	}
	cmd, arg := parseCommand(m.Text)
	var reply string
	switch cmd {
	case "/start":
		if arg == "" {
			reply = msgHello
			break
		}
		username := ""
		if m.From != nil {
			username = m.From.Username
		}
		userID, err := b.Store.ConsumeLinkToken(ctx, linktoken.Hash(arg), m.Chat.ID, username)
		switch {
		case errors.Is(err, storage.ErrNotFound):
			reply = msgBadToken
		case err != nil:
			b.Log.Error("consume link token", "error", err)
			reply = msgInternalErr
		default:
			b.Log.Info("telegram linked", "user_id", userID)
			reply = msgLinked
		}
	case "/stop":
		existed, err := b.Store.DeleteTelegramLinkByChat(ctx, m.Chat.ID)
		switch {
		case err != nil:
			b.Log.Error("unlink telegram chat", "error", err)
			reply = msgInternalErr
		case existed:
			reply = msgUnlinked
		default:
			reply = msgNotLinked
		}
	default:
		reply = msgHelp
	}
	if err := b.API.SendMessage(ctx, m.Chat.ID, reply); err != nil {
		b.Log.Warn("telegram reply failed", "error", err)
	}
}

func parseCommand(text string) (cmd, arg string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	cmd, arg, _ = strings.Cut(text, " ")
	if at := strings.IndexByte(cmd, '@'); at >= 0 {
		cmd = cmd[:at]
	}
	return strings.ToLower(cmd), strings.TrimSpace(arg)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
