package notify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeTelegramAPI struct {
	nextID  int
	deleted []int
	sent    []string
	errSend error
}

func (f *fakeTelegramAPI) Send(ctx context.Context, token, chatID, text string) (SendResult, error) {
	if f.errSend != nil {
		return SendResult{}, f.errSend
	}
	f.sent = append(f.sent, text)
	f.nextID++
	return SendResult{MessageID: f.nextID}, nil
}

func (f *fakeTelegramAPI) Delete(ctx context.Context, token, chatID string, messageID int) error {
	f.deleted = append(f.deleted, messageID)
	return nil
}

func TestTelegramManagerPersistsStateAcrossManagers(t *testing.T) {
	dir := t.TempDir()
	api := &fakeTelegramAPI{nextID: 100}
	m := NewTelegramManager(dir, api)
	if _, err := m.Send(context.Background(), "token", "chat", "scheduler-run-now", "first"); err != nil {
		t.Fatal(err)
	}
	if len(api.deleted) != 0 {
		t.Fatalf("unexpected delete on first send: %v", api.deleted)
	}

	m = NewTelegramManager(dir, api)
	if _, err := m.Send(context.Background(), "token", "chat", "scheduler-run-now", "second"); err != nil {
		t.Fatal(err)
	}
	if len(api.deleted) != 1 || api.deleted[0] != 101 {
		t.Fatalf("expected persisted old message 101 deleted, got %v", api.deleted)
	}
	b, err := os.ReadFile(filepath.Join(dir, "telegram_scheduler-run-now_latest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "second" {
		t.Fatalf("copy mismatch: %q", string(b))
	}
}

func TestTelegramManagerMissingTokenStillSavesCopy(t *testing.T) {
	dir := t.TempDir()
	api := &fakeTelegramAPI{errSend: ErrTelegramSkipped}
	m := NewTelegramManager(dir, api)
	_, err := m.Send(context.Background(), "", "", "x/y", "payload")
	if !errors.Is(err, ErrTelegramSkipped) {
		t.Fatalf("expected skipped err, got %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "telegram_x_y_latest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "payload" {
		t.Fatalf("copy mismatch: %q", string(b))
	}
}

func TestTelegramManagerDedupe(t *testing.T) {
	dir := t.TempDir()
	api := &fakeTelegramAPI{nextID: 100}
	m := NewTelegramManager(dir, api)

	// first send OK
	_, err := m.Send(context.Background(), "token", "chat", "my-alert", "exact text")
	if err != nil {
		t.Fatal(err)
	}
	if len(api.sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(api.sent))
	}

	// second send exact text blocked
	_, err = m.Send(context.Background(), "token", "chat", "my-alert", "exact text")
	if err == nil || !errors.Is(err, ErrTelegramSkipped) {
		t.Fatalf("expected ErrTelegramSkipped due to dedupe, got %v", err)
	}
	if len(api.sent) != 1 {
		t.Fatalf("expected still 1 send, got %d", len(api.sent))
	}

	// third send different text OK
	_, err = m.Send(context.Background(), "token", "chat", "my-alert", "different text")
	if err != nil {
		t.Fatal(err)
	}
	if len(api.sent) != 2 {
		t.Fatalf("expected 2 sends, got %d", len(api.sent))
	}
}
