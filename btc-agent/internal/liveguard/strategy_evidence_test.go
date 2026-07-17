package liveguard

import (
	"btc-agent/internal/config"
	"testing"
)

func TestWithStrategyEvidenceStableAndNoSecrets(t *testing.T) {
	cfg := config.Config{}
	cfg.Live.MaxOrderNotionalUSDT = 10
	d := WithStrategyEvidence(cfg, ManagedDesiredOrder{Symbol: "BTCUSDT"})
	d2 := WithStrategyEvidence(cfg, ManagedDesiredOrder{Symbol: "BTCUSDT"})
	if d.StrategyVersion != HermesStrategyVersion || d.ConfigHash == "" || d.ConfigHash != d2.ConfigHash {
		t.Fatalf("bad evidence %+v %+v", d, d2)
	}
	cfg.Live.MaxOrderNotionalUSDT = 11
	d3 := WithStrategyEvidence(cfg, ManagedDesiredOrder{})
	if d3.ConfigHash == d.ConfigHash {
		t.Fatal("config hash did not change")
	}
}
