package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/llm"
	"btc-agent/internal/research"
	"btc-agent/internal/storage"
)

type expertResearchOutput struct {
	Report   research.ExpertReport    `json:"report"`
	Analysis *research.ExpertAnalysis `json:"analysis,omitempty"`
}

func runExpertResearch(ctx context.Context, cfg config.Config, db *storage.DB, dryRun bool, notifyTelegram bool) error {
	brief := research.BuildBrief(ctx, cfg)
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest market analysis: %w", err)
	}
	plan, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	report := research.ComposeExpertReport(brief, analysis, plan, cfg.Research.ExpertMaxSections, cfg.Research.ExpertMaxItems)
	report.StrategyContext = loadExpertStrategyContext()
	output := expertResearchOutput{Report: report}
	telegramText := deterministicExpertTelegram(report)
	if cfg.AI.Enabled {
		client, clientErr := llm.NewFromEnv(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, cfg.AI.Model, cfg.AI.MaxTokens, cfg.AI.Temperature)
		if clientErr != nil {
			log.Printf("expert research AI client: %v; using deterministic analysis", clientErr)
		} else if ai, aiErr := research.AnalyzeExpertReportWithAI(ctx, client, report); aiErr != nil {
			log.Printf("expert research AI analysis: %v; using deterministic analysis", aiErr)
		} else {
			output.Analysis = &ai
			telegramText = ai.TelegramText
		}
	}
	if err := saveJSONFile("reports", "expert_report_latest.json", output); err != nil {
		return err
	}
	md := report.Markdown()
	if output.Analysis != nil {
		md += "\nCLAW EXPERT ANALYSIS\n\n" + output.Analysis.TelegramText + "\n"
	}
	if err := os.WriteFile(filepath.Join("reports", "expert_report_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if notifyTelegram && !dryRun && cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendScheduledTelegram(ctx, cfg, "expert-report", telegramText)
	}
	fmt.Println(md)
	return nil
}

func deterministicExpertTelegram(report research.ExpertReport) string {
	parts := []string{"📋 BÁO CÁO VĨ MÔ & THỊ TRƯỜNG", "\nKết luận: " + report.Summary}
	for _, section := range report.Sections {
		content := section.Content
		if len(content) > 700 {
			content = content[:697] + "..."
		}
		parts = append(parts, "\n"+section.Title+"\n"+content)
	}
	if len(report.RiskSignals) > 0 {
		risk := make([]string, 0, len(report.RiskSignals))
		for _, signal := range report.RiskSignals {
			risk = append(risk, signal.Level+": "+signal.Detail)
		}
		parts = append(parts, "\n⚠️ Rủi ro\n"+strings.Join(risk, "\n"))
	}
	parts = append(parts, "\nResearch-only: không đặt lệnh, không override Agent 1/2.")
	return strings.Join(parts, "\n")
}

func loadExpertStrategyContext() string {
	b, err := os.ReadFile(filepath.Join("reports", "ai_watch_latest.json"))
	if err != nil {
		return ""
	}
	var ai struct {
		Summary               string   `json:"summary"`
		DeterministicDecision string   `json:"deterministic_decision"`
		Blockers              []string `json:"blockers"`
		WatchTriggers         []string `json:"watch_triggers"`
		RiskWarnings          []string `json:"risk_warnings"`
	}
	if json.Unmarshal(b, &ai) != nil {
		return ""
	}
	parts := []string{}
	if ai.DeterministicDecision != "" {
		parts = append(parts, "decision="+ai.DeterministicDecision)
	}
	if ai.Summary != "" {
		parts = append(parts, ai.Summary)
	}
	if len(ai.Blockers) > 0 {
		parts = append(parts, "blockers: "+strings.Join(ai.Blockers, "; "))
	}
	if len(ai.WatchTriggers) > 0 {
		parts = append(parts, "watch triggers: "+strings.Join(ai.WatchTriggers, "; "))
	}
	if len(ai.RiskWarnings) > 0 {
		parts = append(parts, "risk warnings: "+strings.Join(ai.RiskWarnings, "; "))
	}
	return strings.Join(parts, " | ")
}
