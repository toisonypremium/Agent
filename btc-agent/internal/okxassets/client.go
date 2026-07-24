package okxassets

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const spotBalancePath = "/api/v5/account/balance"

type ReadOnlyClient struct {
	baseURL, key, secret, passphrase string
	http                             *http.Client
	now                              func() time.Time
}

// NewReadOnlyClient deliberately exposes only SpotBalance. It has no trade,
// cancel, transfer, withdrawal, ledger, or runtime write methods.
func NewReadOnlyClient(baseURL, key, secret, passphrase string, httpClient *http.Client, now func() time.Time) *ReadOnlyClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if now == nil {
		now = time.Now
	}
	return &ReadOnlyClient{baseURL: strings.TrimRight(baseURL, "/"), key: key, secret: secret, passphrase: passphrase, http: httpClient, now: now}
}
func (c *ReadOnlyClient) SpotBalance(ctx context.Context) (Snapshot, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Snapshot{}, fmt.Errorf("OKX read-only base URL must be https")
	}
	if strings.TrimSpace(c.key) == "" || strings.TrimSpace(c.secret) == "" || strings.TrimSpace(c.passphrase) == "" {
		return Snapshot{}, fmt.Errorf("OKX read-only credentials missing")
	}
	ts := c.now().UTC().Format("2006-01-02T15:04:05.000Z")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+spotBalancePath, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("OKX balance request: %w", err)
	}
	req.Header.Set("OK-ACCESS-KEY", c.key)
	req.Header.Set("OK-ACCESS-TIMESTAMP", ts)
	req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)
	req.Header.Set("OK-ACCESS-SIGN", sign(ts, http.MethodGet, spotBalancePath, "", c.secret))
	resp, err := c.http.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("OKX balance request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Snapshot{}, fmt.Errorf("OKX balance read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return Snapshot{}, fmt.Errorf("OKX balance HTTP %d", resp.StatusCode)
	}
	return ParseSpotBalance(body)
}
func sign(timestamp, method, path, body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + method + path + body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
