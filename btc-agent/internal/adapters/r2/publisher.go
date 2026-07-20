package r2

import (
	"btc-agent/internal/runtime/outbox"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Publisher writes to a pre-signed S3-compatible PUT URL supplied in the outbox payload destination URL.
// Production signing belongs in a credential provider; frontend never receives it.
type Publisher struct {
	Endpoint string
	Client   *http.Client
}

func (p Publisher) Publish(ctx context.Context, item outbox.Item) error {
	u, err := url.Parse(p.Endpoint)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("R2 HTTPS endpoint required")
	}
	key := u.Query().Get("key")
	if key == "" {
		return fmt.Errorf("R2 deterministic object key required")
	}
	sum := sha256.Sum256(item.Payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.Endpoint, bytes.NewReader(item.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-amz-checksum-sha256", hex.EncodeToString(sum[:]))
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("R2 upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("R2 upload status=%d key=%s", resp.StatusCode, strings.ReplaceAll(key, "..", "_"))
	}
	return nil
}
