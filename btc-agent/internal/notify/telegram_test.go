package notify

import (
	"context"
	"encoding/json"
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

func TestTelegramMissingConfigReturnsError(t *testing.T) {
	secret := "secret-token"
	if err := Telegram(context.Background(), "", "chat", "hello"); err == nil || !strings.Contains(err.Error(), "missing telegram token") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected missing token error: %v", err)
	}
	if err := Telegram(context.Background(), secret, "", "hello"); err == nil || !strings.Contains(err.Error(), "missing telegram chat_id") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected missing chat error: %v", err)
	}
}

func TestTelegramLongMessageSendsChunksInOrder(t *testing.T) {
	texts := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len([]rune(body["text"])) > telegramMaxMessageLen {
			t.Fatalf("chunk too long: %d", len([]rune(body["text"])))
		}
		texts = append(texts, body["text"])
		_, _ = w.Write([]byte(`{"result":{"message_id":123}}`))
	}))
	defer srv.Close()
	oldBase := telegramAPIBaseURL
	telegramAPIBaseURL = srv.URL
	defer func() { telegramAPIBaseURL = oldBase }()

	want := strings.Repeat("a", telegramMaxMessageLen) + strings.Repeat("b", 10) + strings.Repeat("c", telegramMaxMessageLen)
	if err := Telegram(context.Background(), "token", "chat", want); err != nil {
		t.Fatalf("telegram send: %v", err)
	}
	if len(texts) != 3 || strings.Join(texts, "") != want {
		t.Fatalf("chunks not preserved: count=%d", len(texts))
	}
}

func TestTelegramNon2XXDoesNotLeakToken(t *testing.T) {
	secret := "secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad "+secret, http.StatusBadGateway)
	}))
	defer srv.Close()
	oldBase := telegramAPIBaseURL
	telegramAPIBaseURL = srv.URL
	defer func() { telegramAPIBaseURL = oldBase }()

	err := Telegram(context.Background(), secret, "chat", "hello")
	if err == nil || !strings.Contains(err.Error(), "telegram http 502") {
		t.Fatalf("expected non-2xx error: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestTelegramDeleteNon2XXDoesNotLeakToken(t *testing.T) {
	secret := "secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad "+secret, http.StatusBadGateway)
	}))
	defer srv.Close()
	oldBase := telegramAPIBaseURL
	telegramAPIBaseURL = srv.URL
	defer func() { telegramAPIBaseURL = oldBase }()

	err := TelegramDelete(context.Background(), secret, "chat", 123)
	if err == nil || !strings.Contains(err.Error(), "telegram delete http 502") {
		t.Fatalf("expected delete non-2xx error: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("token leaked in delete error: %v", err)
	}
}

func TestTelegramEditNon2XXDoesNotLeakToken(t *testing.T) {
	secret := "secret-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad "+secret, http.StatusBadGateway)
	}))
	defer srv.Close()
	oldBase := telegramAPIBaseURL
	telegramAPIBaseURL = srv.URL
	defer func() { telegramAPIBaseURL = oldBase }()

	err := TelegramEdit(context.Background(), secret, "chat", 123, "hello")
	if err == nil || !strings.Contains(err.Error(), "telegram edit http 502") {
		t.Fatalf("expected edit non-2xx error: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("token leaked in edit error: %v", err)
	}
}

func TestTelegramChunksPreferLineBoundaries(t *testing.T) {
	text := strings.Repeat("a", telegramMaxMessageLen-8) + "\n" + strings.Repeat("b", 20)
	chunks := telegramChunks(text)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if !strings.HasSuffix(chunks[0], "\n") || strings.HasPrefix(chunks[1], "\n") {
		t.Fatalf("chunks should split after newline: %q | %q", chunks[0][len(chunks[0])-5:], chunks[1][:5])
	}
	if strings.Join(chunks, "") != text {
		t.Fatal("chunks did not preserve text")
	}
}
