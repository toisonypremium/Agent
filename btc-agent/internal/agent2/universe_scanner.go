package agent2

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

const (
	UniverseDataOK      = "DATA_OK"
	UniverseDataMissing = "DATA_MISSING"

	UniverseResearchOnly = "Research only: không thay data.symbols.assets, không sửa allocation, không bypass ACTIVE_LIMIT."
)

type UniverseResearchReport struct {
	GeneratedAt      time.Time             `json:"generated_at"`
	ProductionAssets []string              `json:"production_assets"`
	Universe         []string              `json:"universe"`
	Rows             []UniverseResearchRow `json:"rows,omitempty"`
	TopCandidates    []UniverseResearchRow `json:"top_candidates,omitempty"`
	Summary          string                `json:"summary"`
	Safety           string                `json:"safety"`
	ResearchOnly     string                `json:"research_only"`
}

type UniverseResearchRow struct {
	Symbol             string  `json:"symbol"`
	InProduction       bool    `json:"in_production"`
	State              State   `json:"state"`
	TechnicalScore     float64 `json:"technical_score"`
	OpportunityScore   float64 `json:"opportunity_score"`
	OpportunityVerdict string  `json:"opportunity_verdict"`
	SetupScore         float64 `json:"setup_score"`
	RotationRank       int     `json:"rotation_rank,omitempty"`
	RotationScore      float64 `json:"rotation_score,omitempty"`
	RewardRisk         float64 `json:"reward_risk,omitempty"`
	TopBlockerKey      string  `json:"top_blocker_key,omitempty"`
	TopBlocker         string  `json:"top_blocker,omitempty"`
	NextTrigger        string  `json:"next_trigger,omitempty"`
	Reason             string  `json:"reason,omitempty"`
	DataStatus         string  `json:"data_status"`
}

func ResearchUniverseSymbols(cfg config.Config) []string {
	if len(cfg.Data.Symbols.ResearchUniverse) > 0 {
		return uniqueUpperSymbols(cfg.Data.Symbols.ResearchUniverse, cfg.Data.Symbols.BTC)
	}
	return uniqueUpperSymbols(cfg.Data.Symbols.Assets, cfg.Data.Symbols.BTC)
}

func BuildUniverseResearchReport(cfg config.Config, analysis agent1.MarketAnalysis, assets map[string][]market.Candle, benchmark []market.Candle, now time.Time) UniverseResearchReport {
	if now.IsZero() {
		now = time.Now()
	}
	universe := ResearchUniverseSymbols(cfg)
	report := UniverseResearchReport{GeneratedAt: now, ProductionAssets: uniqueUpperSymbols(cfg.Data.Symbols.Assets, cfg.Data.Symbols.BTC), Universe: universe, Safety: "spot limit BUY post-only only; no futures, no leverage, no market order", ResearchOnly: UniverseResearchOnly}
	universeCfg := cfg
	universeCfg.Data.Symbols.Assets = universe
	plan := BuildPlanWithBenchmarks(universeCfg, analysis, assets, map[string][]market.Candle{cfg.Data.Symbols.BTC: benchmark, "BTCUSDT": benchmark})
	assetBySymbol := map[string]AssetPlan{}
	for _, asset := range plan.Assets {
		assetBySymbol[strings.ToUpper(asset.Symbol)] = asset
	}
	production := map[string]bool{}
	for _, symbol := range report.ProductionAssets {
		production[symbol] = true
	}
	for _, symbol := range universe {
		candles := assets[symbol]
		row := UniverseResearchRow{Symbol: symbol, InProduction: production[symbol], State: StateWatch, DataStatus: UniverseDataOK}
		if len(candles) < 60 {
			row.State = StateNoTrade
			row.DataStatus = UniverseDataMissing
			row.TopBlockerKey = EntryCheckData
			row.TopBlocker = "chưa đủ dữ liệu 1D cho universe research"
			row.OpportunityVerdict = OpportunityVerdictData
			row.Reason = row.TopBlocker
			report.Rows = append(report.Rows, row)
			continue
		}
		asset := assetBySymbol[symbol]
		attr := BuildFilterAttribution(asset)
		opp := BuildOpportunityComposite(asset)
		row.State = asset.State
		row.TechnicalScore = asset.SetupScore
		row.OpportunityScore = opp.Score
		row.OpportunityVerdict = opp.Verdict
		row.SetupScore = asset.SetupScore
		row.RotationRank = asset.RotationRank
		row.RotationScore = asset.RotationScore
		row.RewardRisk = asset.RewardRisk
		row.TopBlockerKey = attr.TopBlockerKey
		row.TopBlocker = attr.TopBlocker
		row.NextTrigger = asset.NextTrigger
		row.Reason = firstNonEmptyUniverse(asset.Reason, opp.Reason)
		report.Rows = append(report.Rows, row)
	}
	report.TopCandidates = topUniverseCandidates(report.Rows, 10)
	report.Summary = summarizeUniverseResearch(report)
	return report
}

func topUniverseCandidates(rows []UniverseResearchRow, limit int) []UniverseResearchRow {
	out := append([]UniverseResearchRow(nil), rows...)
	sort.Slice(out, func(i, j int) bool {
		bi := OpportunityCompositeBlocked(out[i].OpportunityVerdict) || out[i].DataStatus != UniverseDataOK
		bj := OpportunityCompositeBlocked(out[j].OpportunityVerdict) || out[j].DataStatus != UniverseDataOK
		if bi != bj {
			return !bi
		}
		if out[i].OpportunityScore != out[j].OpportunityScore {
			return out[i].OpportunityScore > out[j].OpportunityScore
		}
		if out[i].TechnicalScore != out[j].TechnicalScore {
			return out[i].TechnicalScore > out[j].TechnicalScore
		}
		return out[i].Symbol < out[j].Symbol
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func summarizeUniverseResearch(report UniverseResearchReport) string {
	best := UniverseResearchRow{}
	for _, row := range report.TopCandidates {
		if row.DataStatus == UniverseDataOK && !OpportunityCompositeBlocked(row.OpportunityVerdict) {
			best = row
			break
		}
	}
	if best.Symbol == "" {
		return fmt.Sprintf("Universe research symbols=%d candidates=0", len(report.Universe))
	}
	return fmt.Sprintf("Universe research symbols=%d top=%s score=%.1f verdict=%s production=%v", len(report.Universe), best.Symbol, best.OpportunityScore, best.OpportunityVerdict, best.InProduction)
}

func uniqueUpperSymbols(symbols []string, btc string) []string {
	btc = strings.ToUpper(strings.TrimSpace(btc))
	seen := map[string]bool{}
	out := []string{}
	for _, symbol := range symbols {
		s := strings.ToUpper(strings.TrimSpace(symbol))
		if s == "" || s == btc || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func firstNonEmptyUniverse(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
