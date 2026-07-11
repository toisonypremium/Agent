package liveguard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
	"btc-agent/internal/researchprofile"
)

const ShadowProfileArmedProbeLight = "ARMED_PROBE_LIGHT"

type ShadowProbeJournal struct {
	GeneratedAt          time.Time              `json:"generated_at"`
	Profile              string                 `json:"profile"`
	ProductionPermission agent1.Permission      `json:"production_permission"`
	ProductionPlanState  agent2.State           `json:"production_plan_state"`
	ResearchPermission   agent1.Permission      `json:"research_permission"`
	ResearchPlanState    agent2.State           `json:"research_plan_state"`
	DataSanityStatus     string                 `json:"data_sanity_status,omitempty"`
	WouldProbe           bool                   `json:"would_probe"`
	Candidates           []ShadowProbeCandidate `json:"candidates,omitempty"`
	Blockers             []string               `json:"blockers,omitempty"`
	Summary              string                 `json:"summary"`
	Note                 string                 `json:"note"`
}

type ShadowProbeCandidate struct {
	Symbol         string       `json:"symbol"`
	Layer          int          `json:"layer"`
	Entry          float64      `json:"entry"`
	Invalidation   float64      `json:"invalidation"`
	Target         float64      `json:"target"`
	RewardRisk     float64      `json:"reward_risk"`
	Notional       float64      `json:"notional"`
	Quantity       float64      `json:"quantity"`
	State          agent2.State `json:"state"`
	Reason         string       `json:"reason"`
	NextTrigger    string       `json:"next_trigger,omitempty"`
	WouldPlace     bool         `json:"would_place"`
	SetupScore     float64      `json:"setup_score,omitempty"`
	TopBlockerKey  string       `json:"top_blocker_key,omitempty"`
	TopBlocker     string       `json:"top_blocker,omitempty"`
	FailedGates    []string     `json:"failed_gates,omitempty"`
	DiscountGapPct float64      `json:"discount_gap_pct,omitempty"`
	RewardRiskGap  float64      `json:"reward_risk_gap,omitempty"`
	HorizonDays    []int        `json:"horizon_days,omitempty"`
}

func BuildShadowProbeJournal(cfg config.Config, production agent1.MarketAnalysis, productionPlan agent2.Plan, assets map[string][]market.Candle, benchmarks map[string][]market.Candle, dataSanity DataSanityResult, now time.Time) ShadowProbeJournal {
	if now.IsZero() {
		now = time.Now()
	}
	j := ShadowProbeJournal{GeneratedAt: now, Profile: ShadowProfileArmedProbeLight, ProductionPermission: production.ActionPermission, ProductionPlanState: productionPlan.State, DataSanityStatus: dataSanity.Status, Note: "Shadow only — không đặt lệnh thật."}
	profile, ok := researchprofile.ProfileByName(ShadowProfileArmedProbeLight)
	if !ok {
		j.Blockers = []string{"research profile ARMED_PROBE_LIGHT unavailable"}
		j.refreshSummary()
		return j
	}
	j.ResearchPermission = researchprofile.EvaluatePermission(production, profile)
	if j.ResearchPermission != agent1.Armed && j.ResearchPermission != agent1.Allowed {
		j.Blockers = append(j.Blockers, fmt.Sprintf("BTC research profile not ARMED: %s", j.ResearchPermission))
		j.Blockers = append(j.Blockers, production.PermissionReason)
		for _, c := range productionPlan.Watchlist.Candidates {
			j.Blockers = append(j.Blockers, shadowCandidateBlockers(c)...)
		}
		j.refreshSummary()
		return j
	}
	shadowAnalysis := production
	shadowAnalysis.ActionPermission = agent1.Armed
	shadowAnalysis.PermissionReason = "shadow ARMED_PROBE_LIGHT research-only; production permission unchanged"
	shadowPlan := agent2.BuildPlanWithBenchmarks(cfg, shadowAnalysis, assets, benchmarks)
	j.ResearchPlanState = shadowPlan.State
	for _, asset := range shadowPlan.Assets {
		if asset.State != agent2.StateArmed || len(asset.Layers) == 0 {
			if asset.Reason != "" {
				j.Blockers = append(j.Blockers, fmt.Sprintf("%s: %s", asset.Symbol, asset.Reason))
			}
			continue
		}
		layer := asset.Layers[0]
		attr := agent2.BuildFilterAttribution(asset)
		j.Candidates = append(j.Candidates, ShadowProbeCandidate{Symbol: asset.Symbol, Layer: layer.Index, Entry: layer.Price, Invalidation: firstShadowFloat(layer.Invalidation, asset.Invalidation), Target: layer.Target, RewardRisk: firstShadowFloat(layer.RewardRisk, asset.RewardRisk), Notional: layer.Notional, Quantity: layer.Quantity, State: asset.State, Reason: asset.Reason, NextTrigger: asset.NextTrigger, WouldPlace: true, SetupScore: asset.SetupScore, TopBlockerKey: attr.TopBlockerKey, TopBlocker: attr.TopBlocker, FailedGates: failedShadowGates(attr), DiscountGapPct: asset.DiscountGapPct, RewardRiskGap: shadowRewardRiskGap(cfg, asset), HorizonDays: []int{1, 3, 7, 14}})
	}
	if len(j.Candidates) == 0 {
		for _, c := range shadowPlan.Watchlist.Candidates {
			j.Blockers = append(j.Blockers, shadowCandidateBlockers(c)...)
		}
	}
	j.refreshSummary()
	return j
}

// shadowProbeJournalMaxEntries is the maximum number of entries kept in
// shadow_probe_journal.jsonl. Older entries are dropped when the cap is
// exceeded so the file never grows unboundedly during 24/7 operation.
const shadowProbeJournalMaxEntries = 200

func SaveShadowProbeJournal(dir string, j ShadowProbeJournal) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "shadow_probe_latest.json"), b, 0600); err != nil {
		return err
	}
	line, err := json.Marshal(j)
	if err != nil {
		return err
	}
	journalPath := filepath.Join(dir, "shadow_probe_journal.jsonl")
	return appendShadowProbeJournalCapped(journalPath, line, shadowProbeJournalMaxEntries)
}

// appendShadowProbeJournalCapped appends newLine to the journal file and
// trims it to at most maxEntries lines so the file stays bounded.
func appendShadowProbeJournalCapped(path string, newLine []byte, maxEntries int) error {
	// Read existing entries.
	var lines [][]byte
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if l != "" {
				lines = append(lines, []byte(l))
			}
		}
	}
	lines = append(lines, newLine)
	// Trim oldest entries if over cap.
	if len(lines) > maxEntries {
		lines = lines[len(lines)-maxEntries:]
	}
	// Write atomically via temp file.
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	for _, l := range lines {
		if _, err := f.Write(append(l, '\n')); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func LoadShadowProbeLatest(path string) (ShadowProbeJournal, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ShadowProbeJournal{}, false
	}
	var j ShadowProbeJournal
	if err := json.Unmarshal(b, &j); err != nil {
		return ShadowProbeJournal{}, false
	}
	return j, true
}

func (j *ShadowProbeJournal) refreshSummary() {
	j.Blockers = uniqueHealthStrings(j.Blockers)
	j.WouldProbe = len(j.Candidates) > 0
	if j.WouldProbe {
		j.Summary = fmt.Sprintf("SHADOW_%s: would_probe=%d production=%s research=%s; no real order", j.Profile, len(j.Candidates), j.ProductionPermission, j.ResearchPermission)
		return
	}
	j.Summary = fmt.Sprintf("SHADOW_%s: would_probe=0 production=%s research=%s blockers=%d; no real order", j.Profile, j.ProductionPermission, j.ResearchPermission, len(j.Blockers))
}

func shadowCandidateBlockers(c agent2.WatchCandidate) []string {
	out := []string{}
	for _, item := range c.EntryChecklist {
		if !item.Pass && item.Reason != "" {
			out = append(out, fmt.Sprintf("%s %s: %s", c.Symbol, item.Name, item.Reason))
		}
	}
	if len(out) == 0 && len(c.Missing) > 0 {
		for _, m := range c.Missing {
			out = append(out, c.Symbol+": "+m)
		}
	}
	if len(out) == 0 && strings.TrimSpace(c.BlockReason) != "" {
		out = append(out, c.Symbol+": "+c.BlockReason)
	}
	return out
}

func failedShadowGates(attr agent2.FilterAttribution) []string {
	out := []string{}
	for _, gate := range attr.GateRows {
		if !gate.Pass {
			out = append(out, gate.Name)
		}
	}
	return out
}

func shadowRewardRiskGap(cfg config.Config, asset agent2.AssetPlan) float64 {
	if asset.RewardRisk <= 0 || cfg.Risk.MinRewardRisk <= asset.RewardRisk {
		return 0
	}
	return cfg.Risk.MinRewardRisk - asset.RewardRisk
}

func firstShadowFloat(values ...float64) float64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
