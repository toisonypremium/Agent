package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/reportio"
)

const (
	TechnicalVerdictEntryReady   = "ENTRY_READY"
	TechnicalVerdictNearReady    = "NEAR_READY"
	TechnicalVerdictWaitPrice    = "WAIT_PRICE"
	TechnicalVerdictWaitFlow     = "WAIT_FLOW"
	TechnicalVerdictWaitRotation = "WAIT_ROTATION"
	TechnicalVerdictBlockRisk    = "BLOCK_RISK"
	TechnicalVerdictBlockData    = "BLOCK_DATA"
)

type TechnicalScorecardReport struct {
	GeneratedAt   time.Time                `json:"generated_at"`
	PlanState     agent2.State             `json:"plan_state,omitempty"`
	BTCPermission string                   `json:"btc_permission,omitempty"`
	Coins         []TechnicalScorecardCoin `json:"coins,omitempty"`
	Summary       string                   `json:"summary"`
	Safety        string                   `json:"safety"`
}

type TechnicalScorecardCoin struct {
	Symbol         string       `json:"symbol"`
	State          agent2.State `json:"state"`
	TechnicalScore float64      `json:"technical_score"`
	SetupScore     float64      `json:"setup_score"`
	RotationRank   int          `json:"rotation_rank,omitempty"`
	RotationScore  float64      `json:"rotation_score,omitempty"`
	AssetFlowBias  string       `json:"asset_flow_bias,omitempty"`
	AssetFlowScore float64      `json:"asset_flow_score,omitempty"`
	MMCase         string       `json:"mm_case,omitempty"`
	MMScore        float64      `json:"mm_score,omitempty"`
	LiquidityScore float64      `json:"liquidity_score,omitempty"`
	LiquidityGrade string       `json:"liquidity_grade,omitempty"`
	DiscountGapPct float64      `json:"discount_gap_pct,omitempty"`
	ZoneQuality    string       `json:"zone_quality,omitempty"`
	RewardRisk     float64      `json:"reward_risk,omitempty"`
	TopBlockerKey  string       `json:"top_blocker_key,omitempty"`
	TopBlocker     string       `json:"top_blocker,omitempty"`
	FailedHard     int          `json:"failed_hard"`
	FailedSoft     int          `json:"failed_soft"`
	Verdict        string       `json:"verdict"`
	NextTrigger    string       `json:"next_trigger,omitempty"`
	Why            []string     `json:"why,omitempty"`
}

func writeTechnicalScorecardReport(snapshot BotRuntimeSnapshot) error {
	return writeTechnicalScorecardReportFile(buildTechnicalScorecardReport(snapshot))
}

func writeTechnicalScorecardReportFile(report TechnicalScorecardReport) error {
	if err := reportio.WriteJSON("reports", "technical_scorecard_latest.json", report); err != nil {
		return err
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "technical_scorecard_latest.md"), []byte(technicalScorecardMarkdown(report)), 0600)
}

func buildTechnicalScorecardReport(snapshot BotRuntimeSnapshot) TechnicalScorecardReport {
	report := TechnicalScorecardReport{GeneratedAt: snapshot.GeneratedAt, PlanState: snapshot.PlanState, BTCPermission: string(snapshot.BTCPermission), Safety: safetyLine}
	for _, asset := range snapshot.Plan.Assets {
		attr := agent2.BuildFilterAttribution(asset)
		why := append(append([]string{}, asset.HardBlockers...), asset.SoftBlockers...)
		if asset.Reason != "" {
			why = append(why, asset.Reason)
		}
		row := TechnicalScorecardCoin{
			Symbol:         asset.Symbol,
			State:          asset.State,
			TechnicalScore: technicalScore(asset),
			SetupScore:     asset.SetupScore,
			RotationRank:   asset.RotationRank,
			RotationScore:  asset.RotationScore,
			AssetFlowBias:  string(asset.AssetFlowBias),
			AssetFlowScore: asset.AssetFlowScore,
			MMCase:         string(asset.MMCase),
			MMScore:        asset.MMScore,
			LiquidityScore: asset.LiquidityQuality.Score,
			LiquidityGrade: asset.LiquidityQuality.Grade,
			DiscountGapPct: asset.DiscountGapPct,
			ZoneQuality:    asset.ZoneQuality,
			RewardRisk:     asset.RewardRisk,
			TopBlockerKey:  attr.TopBlockerKey,
			TopBlocker:     attr.TopBlocker,
			FailedHard:     attr.FailedHard,
			FailedSoft:     attr.FailedSoft,
			NextTrigger:    asset.NextTrigger,
			Why:            agent2.CompactReasons(why, 5),
		}
		row.Verdict = technicalVerdict(row)
		report.Coins = append(report.Coins, row)
	}
	report.Summary = technicalScorecardSummary(report)
	return report
}

func technicalScore(asset agent2.AssetPlan) float64 {
	score := asset.SetupScore
	if score <= 0 {
		parts := []float64{}
		if asset.RotationScore > 0 {
			parts = append(parts, asset.RotationScore)
		}
		if asset.AssetFlowScore > 0 {
			parts = append(parts, asset.AssetFlowScore)
		}
		if asset.MMScore > 0 {
			parts = append(parts, asset.MMScore/100)
		}
		if asset.LiquidityQuality.Score > 0 {
			parts = append(parts, asset.LiquidityQuality.Score/100)
		}
		for _, part := range parts {
			score += part
		}
		if len(parts) > 0 {
			score /= float64(len(parts))
		}
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func technicalVerdict(row TechnicalScorecardCoin) string {
	key := agent2.NormalizeReasonKey(row.TopBlockerKey)
	if key == agent2.EntryCheckData {
		return TechnicalVerdictBlockData
	}
	if key == agent2.EntryCheckFallingKnife || key == agent2.EntryCheckFOMO || row.FailedHard > 0 {
		return TechnicalVerdictBlockRisk
	}
	if row.State == agent2.StateActiveLimit && row.FailedHard == 0 && row.FailedSoft == 0 {
		return TechnicalVerdictEntryReady
	}
	if row.TechnicalScore >= nearTriggerReadinessThreshold() && row.FailedHard == 0 {
		return TechnicalVerdictNearReady
	}
	switch key {
	case agent2.EntryCheckDiscountZone, agent2.EntryCheckRewardRisk:
		return TechnicalVerdictWaitPrice
	case agent2.EntryCheckMMAccumulation, agent2.EntryCheckAssetFlowEntry:
		return TechnicalVerdictWaitFlow
	case agent2.EntryCheckRotationScore, agent2.EntryCheckRotationRank, agent2.EntryCheckRelativeStrength:
		return TechnicalVerdictWaitRotation
	default:
		if row.FailedSoft > 0 {
			return TechnicalVerdictNearReady
		}
		return TechnicalVerdictWaitPrice
	}
}

func technicalScorecardSummary(report TechnicalScorecardReport) string {
	entry, near, blocked := 0, 0, 0
	best := TechnicalScorecardCoin{}
	for _, coin := range report.Coins {
		if best.Symbol == "" || coin.TechnicalScore > best.TechnicalScore {
			best = coin
		}
		switch coin.Verdict {
		case TechnicalVerdictEntryReady:
			entry++
		case TechnicalVerdictNearReady:
			near++
		case TechnicalVerdictBlockData, TechnicalVerdictBlockRisk:
			blocked++
		}
	}
	if best.Symbol == "" {
		return "Technical scorecard coins=0"
	}
	return fmt.Sprintf("Technical scorecard coins=%d entry_ready=%d near_ready=%d blocked=%d best=%s score=%.0f%%", len(report.Coins), entry, near, blocked, best.Symbol, best.TechnicalScore*100)
}

func technicalScorecardMarkdown(report TechnicalScorecardReport) string {
	var b strings.Builder
	b.WriteString("TECHNICAL SCORECARD\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	b.WriteString("Summary: " + report.Summary + "\n")
	b.WriteString(fmt.Sprintf("Plan state: %s | BTC permission: %s\n\n", report.PlanState, report.BTCPermission))
	for _, coin := range report.Coins {
		b.WriteString(fmt.Sprintf("- %s state=%s score=%.0f%% verdict=%s setup=%.2f rotation=%.2f rank=%d MM=%s %.0f liq=%s %.0f RR=%.2f top=%s\n", coin.Symbol, coin.State, coin.TechnicalScore*100, coin.Verdict, coin.SetupScore, coin.RotationScore, coin.RotationRank, emptyStringDefault(coin.MMCase, "n/a"), coin.MMScore, emptyStringDefault(coin.LiquidityGrade, "n/a"), coin.LiquidityScore, coin.RewardRisk, emptyStringDefault(coin.TopBlockerKey, "none")))
		if len(coin.Why) > 0 {
			b.WriteString("  why=" + strings.Join(firstStrings(coin.Why, 3), "; ") + "\n")
		}
		if coin.NextTrigger != "" {
			b.WriteString("  next=" + coin.NextTrigger + "\n")
		}
	}
	b.WriteString("\nSafety: " + report.Safety + "\n")
	b.WriteString("Research only: scorecard phân tích kỹ thuật; không bypass ACTIVE_LIMIT; WATCH/SCOUT/ARMED không tạo normal live order.\n")
	return b.String()
}
