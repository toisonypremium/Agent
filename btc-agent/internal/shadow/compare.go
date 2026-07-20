package shadow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Comparison struct {
	GeneratedAt         time.Time         `json:"generated_at"`
	RunID               string            `json:"run_id"`
	DecisionMatch       bool              `json:"decision_match"`
	PlanMatch           bool              `json:"plan_match"`
	AllocationMatch     bool              `json:"allocation_match"`
	OrdersMatch         bool              `json:"orders_match"`
	ReconciliationMatch bool              `json:"reconciliation_match"`
	V1Hashes            map[string]string `json:"v1_hashes"`
	V2Hashes            map[string]string `json:"v2_hashes"`
	Mismatches          []string          `json:"mismatches,omitempty"`
}

func Compare(runID string, v1, v2 map[string]any) Comparison {
	c := Comparison{GeneratedAt: time.Now().UTC(), RunID: runID, V1Hashes: map[string]string{}, V2Hashes: map[string]string{}}
	keys := []struct {
		name  string
		match *bool
	}{{"decision", &c.DecisionMatch}, {"plan", &c.PlanMatch}, {"allocation", &c.AllocationMatch}, {"orders", &c.OrdersMatch}, {"reconciliation", &c.ReconciliationMatch}}
	for _, k := range keys {
		a := hash(v1[k.name])
		b := hash(v2[k.name])
		c.V1Hashes[k.name] = a
		c.V2Hashes[k.name] = b
		*k.match = a == b
		if !*k.match {
			c.Mismatches = append(c.Mismatches, fmt.Sprintf("%s mismatch: v1=%s v2=%s", k.name, a, b))
		}
	}
	return c
}
func Pass(c Comparison) bool {
	return c.DecisionMatch && c.PlanMatch && c.AllocationMatch && c.OrdersMatch && c.ReconciliationMatch
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
