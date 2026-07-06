package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var ErrTelegramSkipped = errors.New("telegram skipped: missing token/chat")

var telegramAPIBaseURL = "https://api.telegram.org"

// SendResult holds the message_id returned by Telegram after a successful send.
type SendResult struct {
	MessageID int
}

// Telegram sends a new message and returns the message_id.
func Telegram(ctx context.Context, token, chatID, text string) error {
	_, err := TelegramSend(ctx, token, chatID, text)
	return err
}

// TelegramSend sends a new message and returns message_id + error.
func TelegramSend(ctx context.Context, token, chatID, text string) (SendResult, error) {
	if token == "" || chatID == "" {
		return SendResult{}, ErrTelegramSkipped
	}
	body, _ := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIBaseURL, token), bytes.NewReader(body))
	if err != nil {
		return SendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return SendResult{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return SendResult{}, fmt.Errorf("telegram http %d", resp.StatusCode)
	}
	var raw struct {
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(data, &raw)
	return SendResult{MessageID: raw.Result.MessageID}, nil
}

// TelegramDelete deletes a previously sent message. Errors are non-fatal (message may already be gone).
func TelegramDelete(ctx context.Context, token, chatID string, messageID int) error {
	if token == "" || chatID == "" || messageID == 0 {
		return nil
	}
	body, _ := json.Marshal(map[string]interface{}{"chat_id": chatID, "message_id": messageID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/deleteMessage", telegramAPIBaseURL, token), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("telegram delete http %d: %s", resp.StatusCode, string(data))
	}
	return nil
}

// TelegramEdit edits an existing message. Falls back silently if edit fails.
func TelegramEdit(ctx context.Context, token, chatID string, messageID int, text string) error {
	if token == "" || chatID == "" || messageID == 0 {
		return fmt.Errorf("missing token/chat/messageID")
	}
	body, _ := json.Marshal(map[string]interface{}{"chat_id": chatID, "message_id": messageID, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/editMessageText", telegramAPIBaseURL, token), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("telegram edit http %d", resp.StatusCode)
	}
	return nil
}
