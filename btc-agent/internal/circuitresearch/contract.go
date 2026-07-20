package circuitresearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

const (
	InputSchemaVersion = "circuit-research-input-v1"
	MaxPayloadBytes    = 64 * 1024
)

type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}
type DomainQuality struct {
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
	AgeSeconds  float64   `json:"age_seconds"`
	ReasonCodes []string  `json:"reason_codes,omitempty"`
}
type MarketEvidence struct {
	Venue            string  `json:"venue"`
	InstrumentType   string  `json:"instrument_type"`
	Symbol           string  `json:"symbol"`
	BTCPrice         float64 `json:"btc_price"`
	WeeklyBias       string  `json:"weekly_bias"`
	DailyBias        string  `json:"daily_bias"`
	FourHourBias     string  `json:"four_hour_bias"`
	Regime           string  `json:"regime"`
	TrendScore       float64 `json:"trend_score"`
	RiskLevel        string  `json:"risk_level"`
	FallingKnifeRisk string  `json:"falling_knife_risk"`
	FOMORisk         string  `json:"fomo_risk"`
	Permission       string  `json:"permission"`
	PermissionReason string  `json:"permission_reason"`
	Summary          string  `json:"summary"`
}
type AssetEvidence struct {
	Symbol       string   `json:"symbol"`
	State        string   `json:"state"`
	RewardRisk   float64  `json:"reward_risk"`
	SetupScore   float64  `json:"setup_score"`
	HardBlockers []string `json:"hard_blockers,omitempty"`
	SoftBlockers []string `json:"soft_blockers,omitempty"`
	NextTrigger  string   `json:"next_trigger,omitempty"`
}
type PlanEvidence struct {
	State      string          `json:"state"`
	Permission string          `json:"permission"`
	Summary    string          `json:"summary"`
	Warnings   []string        `json:"warnings,omitempty"`
	Assets     []AssetEvidence `json:"assets"`
}
type AuthorityEvidence struct {
	Verdict     string   `json:"verdict"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}
type Policy struct {
	ResearchOnly       bool `json:"research_only"`
	ExecutionAuthority bool `json:"execution_authority"`
	ShortAllowed       bool `json:"short_allowed"`
	LeverageAllowed    bool `json:"leverage_allowed"`
}
type InputSnapshot struct {
	SchemaVersion      string                   `json:"schema_version"`
	SnapshotID         string                   `json:"snapshot_id"`
	GeneratedAt        time.Time                `json:"generated_at"`
	ValidUntil         time.Time                `json:"valid_until"`
	Producer           Producer                 `json:"producer"`
	Market             MarketEvidence           `json:"market"`
	Plan               PlanEvidence             `json:"plan"`
	DataQuality        map[string]DomainQuality `json:"data_quality"`
	CanonicalAuthority AuthorityEvidence        `json:"canonical_authority"`
	Policy             Policy                   `json:"policy"`
	InputSHA256        string                   `json:"input_sha256"`
}

func BuildInput(analysis agent1.MarketAnalysis, plan agent2.Plan, producer Producer, now time.Time, ttl time.Duration) (InputSnapshot, error) {
	now = now.UTC()
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	status := timestampStatus(analysis.Timestamp, now, ttl)
	reasons := []string{}
	if status == "MISSING" {
		reasons = append(reasons, "CIRCUIT_SOURCE_TIMESTAMP_MISSING")
	} else if status == "STALE" {
		reasons = append(reasons, "CIRCUIT_MARKET_STALE")
	}
	assets := make([]AssetEvidence, 0, len(plan.Assets))
	for _, a := range plan.Assets {
		assets = append(assets, AssetEvidence{Symbol: a.Symbol, State: string(a.State), RewardRisk: a.RewardRisk, SetupScore: a.SetupScore, HardBlockers: append([]string(nil), a.HardBlockers...), SoftBlockers: append([]string(nil), a.SoftBlockers...), NextTrigger: a.NextTrigger})
	}
	verdict := "BLOCKED"
	if analysis.ActionPermission == agent1.Allowed && plan.State == agent2.StateActiveLimit {
		verdict = "ALLOWED"
	}
	out := InputSnapshot{SchemaVersion: InputSchemaVersion, GeneratedAt: now, ValidUntil: now.Add(ttl), Producer: producer, Market: MarketEvidence{Venue: "OKX", InstrumentType: "spot", Symbol: "BTCUSDT", BTCPrice: analysis.BTCPrice, WeeklyBias: analysis.WeeklyBias, DailyBias: analysis.DailyBias, FourHourBias: analysis.FourHourBias, Regime: analysis.MarketRegime, TrendScore: analysis.TrendScore, RiskLevel: string(analysis.RiskLevel), FallingKnifeRisk: string(analysis.FallingKnifeRisk), FOMORisk: string(analysis.FomoRisk), Permission: string(analysis.ActionPermission), PermissionReason: analysis.PermissionReason, Summary: analysis.Summary}, Plan: PlanEvidence{State: string(plan.State), Permission: string(plan.ActionPermission), Summary: plan.Summary, Warnings: append([]string(nil), plan.Warnings...), Assets: assets}, DataQuality: map[string]DomainQuality{"market_analysis": {Status: status, Timestamp: analysis.Timestamp.UTC(), AgeSeconds: nonNegativeAge(now, analysis.Timestamp), ReasonCodes: reasons}, "operations_plan": {Status: timestampStatus(plan.Timestamp, now, ttl), Timestamp: plan.Timestamp.UTC(), AgeSeconds: nonNegativeAge(now, plan.Timestamp)}}, CanonicalAuthority: AuthorityEvidence{Verdict: verdict, ReasonCodes: append([]string(nil), analysis.ScoreBreakdown.RiskBlockers...)}, Policy: Policy{ResearchOnly: true, ExecutionAuthority: false, ShortAllowed: false, LeverageAllowed: false}}
	canonical, err := canonicalInput(out)
	if err != nil {
		return InputSnapshot{}, err
	}
	h := sha256.Sum256(canonical)
	out.InputSHA256 = hex.EncodeToString(h[:])
	out.SnapshotID = "btc-agent:" + out.InputSHA256[:24]
	return out, nil
}
func ValidateInputHash(in InputSnapshot) error {
	canonical, err := canonicalInput(in)
	if err != nil {
		return err
	}
	h := sha256.Sum256(canonical)
	want := hex.EncodeToString(h[:])
	if in.InputSHA256 != want || in.SnapshotID != "btc-agent:"+want[:24] {
		return fmt.Errorf("CIRCUIT_INPUT_HASH_MISMATCH")
	}
	return nil
}
func canonicalInput(in InputSnapshot) ([]byte, error) {
	in.SnapshotID = ""
	in.InputSHA256 = ""
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var canonical any
	if err := json.Unmarshal(raw, &canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}
func timestampStatus(ts, now time.Time, ttl time.Duration) string {
	if ts.IsZero() {
		return "MISSING"
	}
	if now.Sub(ts.UTC()) > ttl {
		return "STALE"
	}
	return "FRESH"
}
func nonNegativeAge(now, ts time.Time) float64 {
	if ts.IsZero() {
		return 0
	}
	age := now.Sub(ts.UTC()).Seconds()
	if age < 0 {
		return 0
	}
	return age
}
