package r2

import (
	"btc-agent/internal/runtime/outbox"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Publisher struct {
	Endpoint, Bucket, AccessKeyID, SecretAccessKey, SessionToken, Region string
	Client                                                               *http.Client
	Now                                                                  func() time.Time
}

func (p Publisher) Publish(ctx context.Context, item outbox.Item) error {
	base, err := url.Parse(strings.TrimRight(p.Endpoint, "/"))
	if err != nil || base.Scheme != "https" {
		return fmt.Errorf("R2 HTTPS endpoint required")
	}
	key := base.Query().Get("key")
	signed := p.AccessKeyID != "" || p.SecretAccessKey != "" || p.Bucket != ""
	if !signed && (item.EventType == "llm_usage" || item.EventType == "llm_usage_daily" || item.EventType == "heartbeat_artifact") {
		if item.EventType == "heartbeat_artifact" {
			key = "heartbeat/latest.json"
		} else {
			key = objectKey(item)
		}
		query := base.Query()
		query.Set("key", key)
		base.RawQuery = query.Encode()
	}
	if signed {
		if p.AccessKeyID == "" || p.SecretAccessKey == "" || p.Bucket == "" {
			return fmt.Errorf("R2 bucket, access key ID and secret access key required")
		}
		if key == "" {
			key = objectKey(item)
		}
		base.RawQuery = ""
		base.Path = "/" + path.Join(p.Bucket, key)
	} else if key == "" {
		return fmt.Errorf("R2 deterministic object key required")
	}
	sum := sha256.Sum256(item.Payload)
	payloadHash := hex.EncodeToString(sum[:])
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, base.String(), bytes.NewReader(item.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-amz-checksum-sha256", payloadHash)
	if signed {
		p.sign(req, payloadHash)
	}
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
func objectKey(i outbox.Item) string {
	at := i.CreatedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	id := i.IdempotencyKey
	if id == "" {
		id = i.ID
	}
	if i.EventType == "llm_usage" {
		return LLMUsageKey(at, id)
	}
	if i.EventType == "llm_usage_daily" {
		return LLMUsageDailyKey(at, id)
	}
	return ReportKey(at, id, "json")
}
func (p Publisher) sign(r *http.Request, payloadHash string) {
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	region := p.Region
	if region == "" {
		region = "auto"
	}
	date := now.Format("20060102")
	amz := now.Format("20060102T150405Z")
	r.Header.Set("x-amz-date", amz)
	r.Header.Set("x-amz-content-sha256", payloadHash)
	if p.SessionToken != "" {
		r.Header.Set("x-amz-security-token", p.SessionToken)
	}
	headers := "content-type:application/octet-stream\nhost:" + r.Host + "\nx-amz-content-sha256:" + payloadHash + "\nx-amz-date:" + amz + "\n"
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonical := r.Method + "\n" + r.URL.EscapedPath() + "\n" + r.URL.Query().Encode() + "\n" + headers + "\n" + signedHeaders + "\n" + payloadHash
	h := sha256.Sum256([]byte(canonical))
	scope := date + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amz + "\n" + scope + "\n" + hex.EncodeToString(h[:])
	kDate := mac([]byte("AWS4"+p.SecretAccessKey), date)
	kRegion := mac(kDate, region)
	kService := mac(kRegion, "s3")
	kSigning := mac(kService, "aws4_request")
	sig := hex.EncodeToString(mac(kSigning, toSign))
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+p.AccessKeyID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+sig)
}
func mac(k []byte, s string) []byte {
	h := hmac.New(sha256.New, k)
	_, _ = h.Write([]byte(s))
	return h.Sum(nil)
}
