package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	BaseURL   string
	APIKey    string
	Model     string
	MaxTokens int
	Temp      float64
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("llm base url required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm api key required")
	}
	if cfg.Model == "" {
		cfg.Model = "Claw"
	}
	if cfg.MaxTokens <= 0 || cfg.MaxTokens > 6000 {
		cfg.MaxTokens = 900
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: 90 * time.Second}}, nil
}

func NewFromEnv(baseEnv, keyEnv, model string, maxTokens int, temp float64) (*Client, error) {
	if baseEnv == "" {
		baseEnv = "ANTHROPIC_BASE_URL"
	}
	if keyEnv == "" {
		keyEnv = "ANTHROPIC_API_KEY"
	}
	if model == "" {
		model = os.Getenv("MODEL")
	}
	return New(Config{BaseURL: os.Getenv(baseEnv), APIKey: os.Getenv(keyEnv), Model: model, MaxTokens: maxTokens, Temp: temp})
}

func (c *Client) ChatJSON(ctx context.Context, prompt string, out any) error {
	content, err := c.ChatText(ctx, prompt)
	if err != nil {
		return err
	}
	content = extractJSONObject(content)
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return fmt.Errorf("llm json parse failed: %w", err)
	}
	return nil
}

func (c *Client) ChatText(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]any{
		"model":       c.cfg.Model,
		"temperature": c.cfg.Temp,
		"messages": []map[string]string{{
			"role":    "user",
			"content": prompt,
		}},
	}
	if c.cfg.MaxTokens > 0 {
		reqBody["max_tokens"] = c.cfg.MaxTokens
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	url := base + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("llm http %d: %s", resp.StatusCode, redact(string(data), c.cfg.APIKey))
	}
	return responseContent(data)
}

func responseContent(data []byte) (string, error) {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("data:")) || bytes.Contains(data, []byte("\ndata:")) {
		return sseContent(data), nil
	}
	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", fmt.Errorf("llm decode failed: %w", err)
	}
	if len(raw.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return raw.Choices[0].Message.Content, nil
}

func sseContent(data []byte) string {
	out := strings.Builder{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			out.WriteString(choice.Delta.Content)
			out.WriteString(choice.Message.Content)
		}
	}
	return out.String()
}

func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "<REDACTED>")
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end >= start {
		return s[start : end+1]
	}
	return s
}
