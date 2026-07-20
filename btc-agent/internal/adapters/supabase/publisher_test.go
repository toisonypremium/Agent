package supabase

import (
	"btc-agent/internal/runtime/outbox"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublisherHeadersIdempotentAndRedacts(t *testing.T) {
	secret := "secret-value"
	var calls int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+secret || !strings.Contains(r.Header.Get("Prefer"), "ignore-duplicates") {
			t.Error("missing auth/idempotency headers")
		}
		w.WriteHeader(500)
		_, _ = w.Write([]byte(secret))
	}))
	defer srv.Close()
	p := Publisher{BaseURL: srv.URL, ServiceKey: secret, Client: srv.Client(), TableForEvent: map[string]string{"decision": "agent_decisions"}, ConflictForEvent: map[string]string{"decision": "id"}}
	err := p.Publish(context.Background(), outbox.Item{EventType: "decision", Payload: []byte(`{"id":"1"}`)})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe err=%v", err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
