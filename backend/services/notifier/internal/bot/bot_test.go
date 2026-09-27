package bot

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"invest/backend/services/notifier/internal/linktoken"
	"invest/backend/services/notifier/internal/storage"
	"invest/backend/services/notifier/internal/telegram"
)

type fakeStore struct {
	mu       sync.Mutex
	tokens   map[string]string
	links    map[int64]string
	username string
}

func (f *fakeStore) ConsumeLinkToken(_ context.Context, hash string, chatID int64, username string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.tokens[hash]
	if !ok {
		return "", storage.ErrNotFound
	}
	delete(f.tokens, hash)
	f.links[chatID] = user
	f.username = username
	return user, nil
}

func (f *fakeStore) DeleteTelegramLinkByChat(_ context.Context, chatID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.links[chatID]
	delete(f.links, chatID)
	return ok, nil
}

type fakeAPI struct {
	mu       sync.Mutex
	updates  []telegram.Update
	served   bool
	replies  map[int64][]string
	offsets  []int64
	sentDone chan struct{}
	expected int
}

func (f *fakeAPI) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/botSECRET/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		method := strings.TrimPrefix(r.URL.Path, "/botSECRET/")

		f.mu.Lock()
		defer f.mu.Unlock()
		var result any
		switch method {
		case "getMe":
			result = telegram.User{ID: 1, IsBot: true, Username: "invest_alerts_bot"}
		case "getUpdates":
			f.offsets = append(f.offsets, int64(params["offset"].(float64)))
			if !f.served {
				f.served = true
				result = f.updates
			} else {
				f.mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				f.mu.Lock()
				result = []telegram.Update{}
			}
		case "sendMessage":
			chat := int64(params["chat_id"].(float64))
			f.replies[chat] = append(f.replies[chat], params["text"].(string))
			f.expected--
			if f.expected == 0 {
				close(f.sentDone)
			}
			result = map[string]any{"message_id": 1}
		default:
			t.Errorf("unexpected method %s", method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	})
}

func msg(id, chat int64, typ, text string) telegram.Update {
	return telegram.Update{UpdateID: id, Message: &telegram.Message{
		Chat: telegram.Chat{ID: chat, Type: typ}, Text: text, From: &telegram.User{ID: chat, Username: "andrey"},
	}}
}

func TestBotFlow(t *testing.T) {
	token, hash, _ := linktoken.Generate()
	store := &fakeStore{tokens: map[string]string{hash: "u1"}, links: map[int64]string{7: "u7"}}
	api := &fakeAPI{
		updates: []telegram.Update{
			msg(100, 10, "private", "/start "+token),
			msg(101, 11, "private", "/start "+token),
			msg(102, 12, "private", "/start"),
			msg(103, 7, "private", "/stop"),
			msg(104, 13, "private", "/stop@invest_alerts_bot"),
			msg(105, 14, "private", "hello"),
			msg(106, 15, "group", "/start "+token),
		},
		replies:  map[int64][]string{},
		sentDone: make(chan struct{}),
		expected: 6,
	}
	srv := httptest.NewServer(api.handler(t))
	defer srv.Close()

	b := &Bot{API: telegram.New(srv.URL, "SECRET"), Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.Run(ctx); close(done) }()

	select {
	case <-api.sentDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for replies")
	}
	for i := 0; i < 200; i++ {
		api.mu.Lock()
		n := len(api.offsets)
		api.mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	if name, ok := b.Username(); !ok || name != "invest_alerts_bot" {
		t.Errorf("username = %q", name)
	}
	if store.links[10] != "u1" || store.username != "andrey" {
		t.Errorf("chat 10 not linked: %v", store.links)
	}
	if _, ok := store.links[7]; ok {
		t.Errorf("chat 7 must be unlinked")
	}
	check := func(chat int64, want string) {
		if len(api.replies[chat]) != 1 || api.replies[chat][0] != want {
			t.Errorf("chat %d replies = %q, want %q", chat, api.replies[chat], want)
		}
	}
	check(10, msgLinked)
	check(11, msgBadToken)
	check(12, msgHello)
	check(7, msgUnlinked)
	check(13, msgNotLinked)
	check(14, msgHelp)
	if len(api.replies[15]) != 0 {
		t.Errorf("group chat must be ignored")
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.offsets) < 2 || api.offsets[0] != 0 || api.offsets[1] != 107 {
		t.Errorf("offsets = %v, want [0 107 ...]", api.offsets)
	}
}

func TestParseCommand(t *testing.T) {
	for in, want := range map[string][2]string{
		"/start abc":           {"/start", "abc"},
		"  /START@Bot  x_y-z ": {"/start", "x_y-z"},
		"/stop":                {"/stop", ""},
		"hi":                   {"", ""},
	} {
		cmd, arg := parseCommand(in)
		if cmd != want[0] || arg != want[1] {
			t.Errorf("parseCommand(%q) = %q, %q", in, cmd, arg)
		}
	}
}
