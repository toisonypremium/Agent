package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramDeleteReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/deleteMessage") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, "message not found", http.StatusBadRequest)
	}))
	defer srv.Close()

	oldBase := telegramAPIBaseURL
	telegramAPIBaseURL = srv.URL
	defer func() { telegramAPIBaseURL = oldBase }()

	err := TelegramDelete(context.Background(), "token", "chat", 123)
	if err == nil || !strings.Contains(err.Error(), "telegram delete http 400") || !strings.Contains(err.Error(), "message not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
