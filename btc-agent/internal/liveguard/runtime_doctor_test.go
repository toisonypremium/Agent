package liveguard

import (
	"strings"
	"testing"
)

func TestRuntimeDoctorRefreshSummaryBlocksAndDedupes(t *testing.T) {
	result := RuntimeDoctorResult{
		Blockers: []string{"missing OKX env", "missing OKX env"},
		Warnings: []string{"telegram missing", "telegram missing"},
	}
	result.RefreshSummary()
	if result.Status != DoctorBlock {
		t.Fatalf("status=%s want %s", result.Status, DoctorBlock)
	}
	if len(result.Blockers) != 1 || len(result.Warnings) != 1 {
		t.Fatalf("expected deduped blockers/warnings: %#v %#v", result.Blockers, result.Warnings)
	}
	if !strings.Contains(result.Summary, "blockers=1") {
		t.Fatalf("summary missing blocker count: %s", result.Summary)
	}
}

func TestRuntimeDoctorSummaryDoesNotIncludeSecretValues(t *testing.T) {
	result := RuntimeDoctorResult{
		AutoLiveEnv:          true,
		CredentialEnvPresent: map[string]bool{"OKX_API_KEY": true, "OKX_API_SECRET": true, "OKX_API_PASSPHRASE": true},
		TelegramTokenPresent: true,
		TelegramChatPresent:  true,
		OKXClientReady:       true,
	}
	result.RefreshSummary()
	for _, leak := range []string{"08ceca61", "276BDF", "@Linh", "telegram_token"} {
		if strings.Contains(result.Summary, leak) {
			t.Fatalf("doctor summary leaked %q: %s", leak, result.Summary)
		}
	}
}
