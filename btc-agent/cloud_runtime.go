package main

import (
	"btc-agent/internal/adapters/r2"
	"btc-agent/internal/adapters/supabase"
	"btc-agent/internal/runtime/outbox"
	"btc-agent/internal/storage"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type cloudRuntime struct {
	worker       outbox.Worker
	destinations []string
	db           *storage.DB
	instance     string
}

func newCloudRuntime(db *storage.DB) (*cloudRuntime, error) {
	pubs := map[string]outbox.Publisher{}
	dest := []string{}
	if u, k := strings.TrimSpace(os.Getenv("SUPABASE_URL")), strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY")); u != "" || k != "" {
		if u == "" || k == "" {
			return nil, fmt.Errorf("both SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY required")
		}
		pubs["supabase"] = supabase.Publisher{BaseURL: u, ServiceKey: k, TableForEvent: map[string]string{"runtime_alert": "runtime_alerts"}, ConflictForEvent: map[string]string{"runtime_alert": "id"}}
		dest = append(dest, "supabase")
	}
	if u := strings.TrimSpace(os.Getenv("R2_PRESIGNED_PUT_URL")); u != "" {
		pubs["r2"] = r2.Publisher{Endpoint: u}
		dest = append(dest, "r2")
	} else {
		endpoint, bucket := strings.TrimSpace(os.Getenv("R2_ENDPOINT")), strings.TrimSpace(os.Getenv("R2_BUCKET"))
		access, secret := strings.TrimSpace(os.Getenv("R2_ACCESS_KEY_ID")), strings.TrimSpace(os.Getenv("R2_SECRET_ACCESS_KEY"))
		if endpoint != "" || bucket != "" || access != "" || secret != "" {
			if endpoint == "" || bucket == "" || access == "" || secret == "" {
				return nil, fmt.Errorf("R2_ENDPOINT, R2_BUCKET, R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY are all required")
			}
			pubs["r2"] = r2.Publisher{Endpoint: endpoint, Bucket: bucket, AccessKeyID: access, SecretAccessKey: secret, SessionToken: strings.TrimSpace(os.Getenv("R2_SESSION_TOKEN")), Region: "auto"}
			dest = append(dest, "r2")
		}
	}
	instance := strings.TrimSpace(os.Getenv("BTC_AGENT_INSTANCE_ID"))
	if instance == "" {
		instance = fmt.Sprintf("pid-%d", os.Getpid())
	}
	return &cloudRuntime{worker: outbox.Worker{Store: db, Publishers: pubs, MaxRetries: 8}, destinations: dest, db: db, instance: instance}, nil
}
func (c *cloudRuntime) run(ctx context.Context) {
	if len(c.destinations) == 0 {
		log.Printf("[CloudRuntime] disabled: no Supabase/R2 credentials configured")
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	emit := time.NewTicker(time.Minute)
	defer emit.Stop()
	c.enqueue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.worker.RunOnce(ctx, 20); err != nil {
				log.Printf("[CloudRuntime] outbox worker error: %v", err)
			}
		case <-emit.C:
			c.enqueue(ctx)
		}
	}
}
func (c *cloudRuntime) enqueue(ctx context.Context) {
	now := time.Now().UTC()
	id := uuidV4()
	payload, _ := json.Marshal(map[string]any{"id": id, "severity": "INFO", "category": "HEARTBEAT", "message": "V2 runtime heartbeat instance=" + c.instance, "created_at": now.Format(time.RFC3339)})
	for _, d := range c.destinations {
		body := payload
		event := "runtime_alert"
		if d == "r2" {
			event = "heartbeat_artifact"
		}
		item := outbox.Item{ID: uuidV4(), EventType: event, Destination: d, Payload: body, IdempotencyKey: d + ":" + id, CreatedAt: now}
		if err := c.db.EnqueueOutbox(ctx, item); err != nil {
			log.Printf("[CloudRuntime] enqueue %s error: %v", d, err)
		}
	}
}
func uuidV4() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}
