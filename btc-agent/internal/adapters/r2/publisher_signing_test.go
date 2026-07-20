package r2

import (
	"btc-agent/internal/runtime/outbox"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublisherSigV4(t *testing.T) {
	var gotPath, auth string
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		auth = r.Header.Get("Authorization")
		if r.Header.Get("x-amz-content-sha256") == "" || r.Header.Get("x-amz-date") == "" {
			t.Error("missing signing headers")
		}
		w.WriteHeader(200)
	}))
	defer s.Close()
	p := Publisher{Endpoint: s.URL, Bucket: "bucket", AccessKeyID: "access", SecretAccessKey: "secret", Client: s.Client(), Now: func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC) }}
	i := outbox.Item{ID: "item", IdempotencyKey: "heartbeat:1", Payload: []byte("x"), CreatedAt: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)}
	if err := p.Publish(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=access/") {
		t.Fatalf("bad auth %q", auth)
	}
	if !strings.HasPrefix(gotPath, "/bucket/reports/2026/07/20/") {
		t.Fatalf("bad path %q", gotPath)
	}
}
func TestPublisherRejectsPartialCredentials(t *testing.T) {
	p := Publisher{Endpoint: "https://example.com", Bucket: "b"}
	if err := p.Publish(context.Background(), outbox.Item{}); err == nil {
		t.Fatal("expected error")
	}
}
