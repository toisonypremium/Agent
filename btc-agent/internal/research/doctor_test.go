package research

import (
	"context"
	"strings"
	"testing"

	"btc-agent/internal/config"
)

func TestDoctorDisabledWarns(t *testing.T) {
	result := RunDoctor(context.Background(), config.Config{})
	if result.Status != StatusWarn {
		t.Fatalf("status=%s want %s", result.Status, StatusWarn)
	}
	if !strings.Contains(result.Summary, "warnings=1") {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
}

func TestDoctorNoUsableChannelBlocks(t *testing.T) {
	cfg := config.Config{}
	cfg.Research.Enabled = true
	cfg.Research.RequestTimeoutSeconds = 1
	cfg.Research.RSS.Enabled = true
	cfg.Research.RSS.Feeds = []string{"http://127.0.0.1:1/nope"}
	result := RunDoctor(context.Background(), cfg)
	if result.Status != StatusBlock {
		t.Fatalf("status=%s want %s", result.Status, StatusBlock)
	}
	for _, leak := range []string{"OKX_API_SECRET", "telegram_token", "08ceca61"} {
		if strings.Contains(result.Summary, leak) {
			t.Fatalf("summary leaked %q: %s", leak, result.Summary)
		}
	}
}
