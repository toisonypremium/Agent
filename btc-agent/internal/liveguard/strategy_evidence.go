package liveguard

import (
	"btc-agent/internal/config"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const HermesStrategyVersion = "hermes-spot-lifecycle-v1"

// WithStrategyEvidence adds immutable software/config identity before an order
// is reserved. Only a hash is persisted; credentials and config contents are not.
func WithStrategyEvidence(cfg config.Config, d ManagedDesiredOrder) ManagedDesiredOrder {
	d.StrategyVersion = HermesStrategyVersion
	b, _ := json.Marshal(cfg)
	sum := sha256.Sum256(b)
	d.ConfigHash = hex.EncodeToString(sum[:])[:16]
	return d
}
