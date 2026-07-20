package r2

import (
	"btc-agent/internal/runtime/outbox"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
