package r2

import (
	"btc-agent/internal/runtime/outbox"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublisherChecksumAndPut(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.Header.Get("x-amz-checksum-sha256") == "" {
			t.Error("missing PUT/checksum")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	p := Publisher{Endpoint: srv.URL + "?key=reports/2026/x.json", Client: srv.Client()}
	if err := p.Publish(context.Background(), outbox.Item{Payload: []byte("x")}); err != nil {
		t.Fatal(err)
	}
}

func TestPresignedPublisherOverridesKeyForUsageArtifacts(t *testing.T) {
	var gotKey string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	p := Publisher{Endpoint: srv.URL + "?token=secret&key=heartbeat/latest.json", Client: srv.Client()}
	item := outbox.Item{ID: "request-1", EventType: "llm_usage", IdempotencyKey: "r2:llm_usage:request-1", Payload: []byte("{}"), CreatedAt: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)}
	if err := p.Publish(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	want := "llm-usage/2026/07/21/events/r2-llm_usage-request-1.json"
	if gotKey != want {
		t.Fatalf("key=%q want=%q", gotKey, want)
	}
}

func TestPresignedPublisherKeepsHeartbeatKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	p := Publisher{Endpoint: srv.URL + "?token=secret&key=old.json", Client: srv.Client()}
	if err := p.Publish(context.Background(), outbox.Item{EventType: "heartbeat_artifact", Payload: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if gotKey != "heartbeat/latest.json" {
		t.Fatalf("heartbeat key=%q", gotKey)
	}
}
