package llm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Usage struct {
	PromptTokens     int  `json:"prompt_tokens,omitempty"`
	CompletionTokens int  `json:"completion_tokens,omitempty"`
	TotalTokens      int  `json:"total_tokens,omitempty"`
	Available        bool `json:"available"`
}

type CallResult struct {
	RequestID     string        `json:"request_id"`
	Timestamp     time.Time     `json:"timestamp"`
	Purpose       string        `json:"purpose"`
	TriggerSource string        `json:"trigger_source,omitempty"`
	TriggerReason string        `json:"trigger_reason,omitempty"`
	StateHash     string        `json:"state_hash,omitempty"`
	Model         string        `json:"model"`
	Usage         Usage         `json:"usage"`
	Latency       time.Duration `json:"-"`
	LatencyMS     int64         `json:"latency_ms"`
	Status        string        `json:"status"`
	ErrorClass    string        `json:"error_class,omitempty"`
}

type Observer func(CallResult)

type Config struct {
	BaseURL       string
	APIKey        string
	Model         string
	MaxTokens     int
	Temp          float64
	Purpose       string
	TriggerSource string
	TriggerReason string
	StateHash     string
	Observer      Observer
}

type Client struct {
	cfg  Config
	http *http.Client
}

const maxResponseBytes = 2 << 20

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
	return NewFromEnvConfig(baseEnv, keyEnv, Config{Model: model, MaxTokens: maxTokens, Temp: temp})
}

func NewFromEnvConfig(baseEnv, keyEnv string, cfg Config) (*Client, error) {
	if baseEnv == "" {
		baseEnv = "ANTHROPIC_BASE_URL"
	}
	if keyEnv == "" {
		keyEnv = "ANTHROPIC_API_KEY"
	}
	if cfg.Model == "" {
		cfg.Model = os.Getenv("MODEL")
	}
	cfg.BaseURL = os.Getenv(baseEnv)
	cfg.APIKey = os.Getenv(keyEnv)
	return New(cfg)
}

func (c *Client) ChatJSON(ctx context.Context, prompt string, out any) error {
	content, result, err := c.chat(ctx, prompt)
	if err != nil {
		c.observe(result)
		return err
	}
	if err := decodeStrictJSONObject(content, out); err != nil {
		result.Status = "error"
		result.ErrorClass = "json_parse"
		c.observe(result)
		return fmt.Errorf("llm json parse failed: %w", err)
	}
	c.observe(result)
	return nil
}

func (c *Client) ChatText(ctx context.Context, prompt string) (string, error) {
	content, result, err := c.chat(ctx, prompt)
	c.observe(result)
	return content, err
}

func (c *Client) chat(ctx context.Context, prompt string) (string, CallResult, error) {
	started := time.Now()
	result := CallResult{RequestID: requestID(), Timestamp: started.UTC(), Purpose: c.cfg.Purpose, TriggerSource: c.cfg.TriggerSource, TriggerReason: c.cfg.TriggerReason, StateHash: c.cfg.StateHash, Model: c.cfg.Model, Status: "ok"}
	finish := func(class string, err error) (string, CallResult, error) {
		result.Latency = time.Since(started)
		result.LatencyMS = result.Latency.Milliseconds()
		if err != nil {
			result.Status = "error"
			result.ErrorClass = class
		}
		return "", result, err
	}
	reqBody := map[string]any{"model": c.cfg.Model, "temperature": c.cfg.Temp, "messages": []map[string]string{{"role": "system", "content": "You are a report-only assistant. Never override the deterministic engine, never place orders, and return only the requested format."}, {"role": "user", "content": prompt}}}
	if c.cfg.MaxTokens > 0 {
		reqBody["max_tokens"] = c.cfg.MaxTokens
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return finish("request", err)
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return finish("request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		class := "request"
		if ctx.Err() != nil {
			class = "context"
		}
		_, result, _ = finish(class, err)
		return "", result, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return finish("decode", err)
	}
	if len(data) > maxResponseBytes {
		return finish("response_too_large", fmt.Errorf("llm response exceeds %d bytes", maxResponseBytes))
	}
	if resp.StatusCode/100 != 2 {
		class := "http_4xx"
		if resp.StatusCode >= 500 {
			class = "http_5xx"
		}
		return finish(class, fmt.Errorf("llm http %d", resp.StatusCode))
	}
	content, usage, err := responseContent(data)
	result.Usage = usage
	result.Latency = time.Since(started)
	result.LatencyMS = result.Latency.Milliseconds()
	if err != nil {
		result.Status = "error"
		result.ErrorClass = "decode"
		return "", result, err
	}
	return content, result, nil
}

func (c *Client) observe(result CallResult) {
	if c.cfg.Observer != nil {
		c.cfg.Observer(result)
	}
}

func responseContent(data []byte) (string, Usage, error) {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("data:")) || bytes.Contains(data, []byte("\ndata:")) {
		content, usage := sseContent(data)
		return content, usage, nil
	}
	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", Usage{}, fmt.Errorf("llm decode failed: %w", err)
	}
	if len(raw.Choices) == 0 {
		return "", usageFromRaw(raw.Usage), fmt.Errorf("llm returned no choices")
	}
	return raw.Choices[0].Message.Content, usageFromRaw(raw.Usage), nil
}

func usageFromRaw(raw *struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}) Usage {
	if raw == nil {
		return Usage{Available: false}
	}
	return Usage{PromptTokens: raw.PromptTokens, CompletionTokens: raw.CompletionTokens, TotalTokens: raw.TotalTokens, Available: true}
}

func sseContent(data []byte) (string, Usage) {
	var out strings.Builder
	usage := Usage{Available: false}
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
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		if chunk.Usage != nil {
			usage = usageFromRaw(chunk.Usage)
		}
		for _, choice := range chunk.Choices {
			out.WriteString(choice.Delta.Content)
			out.WriteString(choice.Message.Content)
		}
	}
	return out.String(), usage
}

func decodeStrictJSONObject(s string, out any) error {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "```json"), "```"))
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "```"), "```"))
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing content")
	}
	return nil
}

func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("llm-%d", time.Now().UnixNano())
	}
	return "llm-" + hex.EncodeToString(b[:])
}
