package supabase

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"btc-agent/internal/runtime/outbox"
)

type Publisher struct {
	BaseURL, ServiceKey string
	Client              *http.Client
	TableForEvent       map[string]string
}

func (p Publisher) Publish(ctx context.Context, item outbox.Item) error {
	base, err := url.Parse(strings.TrimRight(p.BaseURL, "/"))
	if err != nil || base.Scheme != "https" {
		return fmt.Errorf("supabase HTTPS URL required")
	}
	if p.ServiceKey == "" {
		return fmt.Errorf("supabase service key required")
	}
	table := p.TableForEvent[item.EventType]
	if table == "" {
		return fmt.Errorf("unsupported supabase event type %q", item.EventType)
	}
	base.Path = "/rest/v1/" + url.PathEscape(table)
	q := base.Query()
	q.Set("on_conflict", "id")
	base.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(item.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", p.ServiceKey)
	req.Header.Set("Authorization", "Bearer "+p.ServiceKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=ignore-duplicates,return=minimal")
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("supabase publish: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		safe := strings.ReplaceAll(string(body), p.ServiceKey, "<REDACTED>")
		return fmt.Errorf("supabase publish status=%d body=%s", resp.StatusCode, safe)
	}
	return nil
}
