package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/hermesagent"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/storage"
)

const hermesReportDir = "reports"

// runHermesCycle builds HermesSnapshot from report files, calls LLM, writes report, sends Telegram.
func runHermesCycle(ctx context.Context, cfg config.Config, db *storage.DB) error {
	snap := buildHermesSnapshot(cfg)
	var caller hermesagent.JSONCaller
	if cfg.AI.Enabled {
		client, err := llm.NewFromEnv(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, cfg.AI.Model, cfg.AI.MaxTokens, cfg.AI.Temperature)
		if err != nil {
			log.Printf("[Hermes] LLM client warning: %v", err)
		} else {
			caller = client
		}
	}
	report, err := hermesagent.Generate(ctx, caller, snap)
	if err != nil {
		log.Printf("[Hermes] LLM warning: %v", err)
	}
	if err := saveJSONFile(hermesReportDir, "hermes_report_latest.json", report); err != nil {
		return fmt.Errorf("hermes report save: %w", err)
	}
	md := buildHermesMarkdown(snap, report)
	if err := os.MkdirAll(hermesReportDir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(hermesReportDir, "hermes_report_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && report.WorthyAlert {
		sendTelegram(ctx, cfg, "hermes-cycle", report.TelegramText)
	}
	return nil
}

// buildHermesSnapshot assembles state from all available report files.
func buildHermesSnapshot(cfg config.Config) hermesagent.HermesSnapshot {
	snap := hermesagent.HermesSnapshot{
		GeneratedAt: time.Now().UTC(),
	}

	// ── Audit report ──────────────────────────────────────────────
	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "live_auto_audit_latest.json")); err == nil {
		var audit struct {
			GeneratedAt          time.Time `json:"generated_at"`
			Verdict              string    `json:"verdict"`
			CurrentMarketAuth    string    `json:"current_market_authority"`
			CurrentDryRunApproved bool     `json:"current_dry_run_approved"`
			Reasons              []string  `json:"reasons"`
			ForcedSimulation     struct {
				Passed bool `json:"passed"`
			} `json:"forced_simulation"`
			Doctor struct {
				Status   string   `json:"status"`
				Blockers []string `json:"blockers"`
			} `json:"doctor"`
			Analysis struct {
				ActionPermission string `json:"action_permission"`
				MarketRegime     string `json:"market_regime"`
				TrendScore       float64 `json:"trend_score"`
				BTCAccumulation  struct {
					Phase string `json:"phase"`
				} `json:"btc_accumulation"`
			} `json:"analysis"`
			Plan struct {
				State string `json:"state"`
			} `json:"plan"`
		}
		if json.Unmarshal(b, &audit) == nil {
			snap.AuditVerdict = audit.Verdict
			snap.MarketAuthority = audit.CurrentMarketAuth
			snap.CurrentDryRunApproved = audit.CurrentDryRunApproved
			snap.ForcedSimPassed = audit.ForcedSimulation.Passed
			snap.AuditReasons = audit.Reasons
			snap.BTCPermission = audit.Analysis.ActionPermission
			snap.BTCRegime = audit.Analysis.MarketRegime
			snap.BTCTrend = audit.Analysis.TrendScore
			snap.BTCPhase = audit.Analysis.BTCAccumulation.Phase
			snap.PlanState = audit.Plan.State
			snap.DoctorStatus = audit.Doctor.Status
			snap.DoctorBlockers = audit.Doctor.Blockers
			if !audit.GeneratedAt.IsZero() {
				age := time.Since(audit.GeneratedAt)
				snap.AuditAge = fmt.Sprintf("%.0f", math.Round(age.Minutes()))
			}
		}
	}

	// ── Supervisor report (exits + operator halt) ──────────────────
	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "live_supervisor_latest.json")); err == nil {
		var sup liveguard.SupervisorResult
		if json.Unmarshal(b, &sup) == nil {
			snap.OperatorHalted = sup.AutoHalted
			snap.ExitEnabled = len(sup.Exits) > 0
			snap.LastSupervisorAt = sup.GeneratedAt.Format(time.RFC3339)
			for _, ex := range sup.Exits {
				snap.Exits = append(snap.Exits, hermesagent.HermesExit{
					Symbol: ex.Symbol,
					Action: string(ex.Action),
					PnLPct: ex.PnLPct,
					Reason: ex.Reason,
				})
			}
		}
	}

	// ── Scenario report (assets) ──────────────────────────────────
	if scenario, ok := loadScenarioReportFile(); ok {
		for _, coin := range scenario.Coins {
			why := ""
			if len(coin.WhyNoOrder) > 0 {
				why = strings.Join(coin.WhyNoOrder, "; ")
				if len(why) > 120 {
					why = why[:120] + "..."
				}
			}
			snap.Assets = append(snap.Assets, hermesagent.HermesAsset{
				Symbol:     coin.Symbol,
				State:      string(coin.State),
				Readiness:  coin.ReadinessScore * 100,
				RR:         coin.RewardRisk,
				OpenOrders: coin.OpenOrders,
				Why:        why,
			})
		}
	}

	// ── Live positions ────────────────────────────────────────────
	if posReport, ok := loadLivePositionReportFile(); ok {
		for _, pos := range posReport.Positions {
			snap.Positions = append(snap.Positions, hermesagent.HermesPosition{
				Symbol:        pos.Symbol,
				Quantity:      pos.Quantity,
				AvgEntryPrice: pos.AvgEntryPrice,
				OpenedAt:      pos.OpenedAt,
			})
		}
	}

	// ── Research brief ────────────────────────────────────────────
	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "research_brief_latest.json")); err == nil {
		var brief struct {
			Summary string `json:"summary"`
		}
		if json.Unmarshal(b, &brief) == nil && brief.Summary != "" {
			snap.ResearchSummary = brief.Summary
			if len(snap.ResearchSummary) > 300 {
				snap.ResearchSummary = snap.ResearchSummary[:300] + "..."
			}
		}
	}

	// ── Scheduler heartbeat ───────────────────────────────────────
	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "scheduler_heartbeat.json")); err == nil {
		var hb struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(b, &hb) == nil {
			snap.SchedulerRunning = hb.Status == "running"
		}
	}

	return snap
}

func buildHermesMarkdown(snap hermesagent.HermesSnapshot, report hermesagent.HermesReport) string {
	var b strings.Builder
	b.WriteString("HERMES BOT MANAGER\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Audit age: %s min\n\n", snap.AuditAge))

	b.WriteString("📊 STRATEGY\n")
	b.WriteString(fmt.Sprintf("BTC Phase: %s | Permission: %s | Regime: %s | Trend: %.1f\n",
		snap.BTCPhase, snap.BTCPermission, snap.BTCRegime, snap.BTCTrend))
	b.WriteString(fmt.Sprintf("Audit: %s | Market authority: %s\n", snap.AuditVerdict, snap.MarketAuthority))
	b.WriteString(fmt.Sprintf("Doctor: %s\n", snap.DoctorStatus))
	if len(snap.DoctorBlockers) > 0 {
		b.WriteString(fmt.Sprintf("Blockers: %s\n", strings.Join(snap.DoctorBlockers, "; ")))
	}
	b.WriteString("\n")

	if len(snap.Assets) > 0 {
		b.WriteString("📈 ASSETS\n")
		for _, a := range snap.Assets {
			b.WriteString(fmt.Sprintf("- %s: %s readiness=%.0f%% RR=%.2f orders=%d\n",
				a.Symbol, a.State, a.Readiness, a.RR, a.OpenOrders))
		}
		b.WriteString("\n")
	}

	if len(snap.Exits) > 0 {
		b.WriteString("📉 EXIT SIGNALS\n")
		for _, ex := range snap.Exits {
			b.WriteString(fmt.Sprintf("- %s → %s PnL=%.2f%%: %s\n",
				ex.Symbol, ex.Action, ex.PnLPct*100, ex.Reason))
		}
		b.WriteString("⚠ Report-only — PlaceSellLimitOrder not auto-called.\n\n")
	} else {
		b.WriteString("📉 EXIT SIGNALS: NONE\n\n")
	}

	if len(snap.Positions) > 0 {
		b.WriteString("💼 POSITIONS\n")
		for _, p := range snap.Positions {
			b.WriteString(fmt.Sprintf("- %s qty=%.6f avg=%.4f\n",
				p.Symbol, p.Quantity, p.AvgEntryPrice))
		}
		b.WriteString("\n")
	}

	b.WriteString("🤖 HERMES ANALYSIS\n")
	b.WriteString(fmt.Sprintf("Gate: %s\n", report.GateSummary))
	b.WriteString(fmt.Sprintf("Assets: %s\n", report.AssetSummary))
	b.WriteString(fmt.Sprintf("Exits: %s\n", report.ExitSummary))
	if len(report.Anomalies) > 0 {
		b.WriteString(fmt.Sprintf("⚠ Anomalies: %s\n", strings.Join(report.Anomalies, "; ")))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("✅ %s\n", report.ActionLine))
	return b.String()
}

// loadHermesReportFile loads the latest hermes report for Telegram command.
func loadHermesReportFile() (hermesagent.HermesReport, bool) {
	b, err := os.ReadFile(filepath.Join(hermesReportDir, "hermes_report_latest.json"))
	if err != nil {
		return hermesagent.HermesReport{}, false
	}
	var out hermesagent.HermesReport
	if err := json.Unmarshal(b, &out); err != nil {
		return hermesagent.HermesReport{}, false
	}
	return out, true
}
