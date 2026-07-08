package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const telegramMaxMessageLen = 4000

var ErrTelegramConfig = errors.New("telegram config error")
var ErrTelegramSkipped = ErrTelegramConfig

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
	if strings.TrimSpace(token) == "" {
		return SendResult{}, fmt.Errorf("%w: missing telegram token", ErrTelegramConfig)
	}
	if strings.TrimSpace(chatID) == "" {
		return SendResult{}, fmt.Errorf("%w: missing telegram chat_id", ErrTelegramConfig)
	}
	chunks := telegramChunks(text)
	var result SendResult
	for _, chunk := range chunks {
		sent, err := telegramSendChunk(ctx, token, chatID, chunk)
		if err != nil {
			return result, err
		}
		result = sent
	}
	return result, nil
}

func telegramSendChunk(ctx context.Context, token, chatID, text string) (SendResult, error) {
	body, _ := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIBaseURL, token), bytes.NewReader(body))
	if err != nil {
		return SendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, data, err := doWithRetry(req)
	if err != nil {
		return SendResult{}, err
	}
	if resp.StatusCode/100 != 2 {
		return SendResult{}, fmt.Errorf("telegram http %d: %s", resp.StatusCode, telegramRedact(string(bytes.TrimSpace(data)), token))
	}
	var raw struct {
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(data, &raw)
	return SendResult{MessageID: raw.Result.MessageID}, nil
}

func doWithRetry(req *http.Request) (*http.Response, []byte, error) {
	client := http.Client{Timeout: 15 * time.Second}
	var lastErr error
	var resp *http.Response
	var data []byte

	backoff := 1 * time.Second
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		// Clone request body for retry
		var bodyReader io.Reader
		if req.GetBody != nil {
			bodyReadCloser, _ := req.GetBody()
			bodyReader = bodyReadCloser
		} else if req.Body != nil {
			buf, _ := io.ReadAll(req.Body)
			req.Body.Close()
			req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(buf)), nil }
			bodyReader = bytes.NewReader(buf)
		}

		clone := req.Clone(req.Context())
		if bodyReader != nil {
			clone.Body = io.NopCloser(bodyReader)
		}

		var err error
		resp, err = client.Do(clone)
		if err != nil {
			lastErr = err
			continue
		}

		data, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(bytes.TrimSpace(data)))
			continue
		}
		return resp, data, nil
	}
	return resp, data, lastErr
}

func telegramChunks(text string) []string {
	if text == "" {
		return []string{""}
	}
	runes := []rune(text)
	out := []string{}
	for len(runes) > 0 {
		end := telegramMaxMessageLen
		if len(runes) < end {
			end = len(runes)
		} else {
			for i := end - 1; i > 0; i-- {
				if runes[i] == '\n' {
					end = i + 1
					break
				}
			}
		}
		out = append(out, string(runes[:end]))
		runes = runes[end:]
	}
	return out
}

func telegramRedact(text, token string) string {
	if token != "" {
		text = strings.ReplaceAll(text, token, "<REDACTED>")
	}
	if len(text) > 500 {
		text = text[:500] + "..."
	}
	return text
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
		return fmt.Errorf("telegram delete http %d: %s", resp.StatusCode, telegramRedact(string(bytes.TrimSpace(data)), token))
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
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("telegram edit http %d: %s", resp.StatusCode, telegramRedact(string(bytes.TrimSpace(data)), token))
	}
	return nil
}
