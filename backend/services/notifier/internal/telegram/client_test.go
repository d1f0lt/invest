package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestErrorsAreDecodedAndTokenNotLeaked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`))
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "123:SECRET")
	err := c.SendMessage(context.Background(), 1, "hi")
	if !IsBlocked(err) {
		t.Errorf("expected blocked, got %v", err)
	}

	_, err = c.GetMe(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != 7*time.Second || IsBlocked(err) {
		t.Errorf("unexpected: %v", err)
	}

	bad := New("http://127.0.0.1:1", "123:SECRET")
	err = bad.SendMessage(context.Background(), 1, "hi")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("transport error must not contain the token: %v", err)
	}
}

func TestIsBlockedChatNotFound(t *testing.T) {
	if !IsBlocked(&APIError{Code: 400, Description: "Bad Request: chat not found"}) {
		t.Error("chat not found must count as blocked")
	}
	if IsBlocked(&APIError{Code: 400, Description: "Bad Request: message is too long"}) {
		t.Error("other 400s are not blocked")
	}
}
