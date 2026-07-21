package main

import (
	"btc-agent/internal/aiagent"
	"btc-agent/internal/aieval"
	"btc-agent/internal/config"
	"btc-agent/internal/notify"
	"btc-agent/internal/research"
	"btc-agent/internal/storage"
	"btc-agent/internal/telegramreport"
	"btc-agent/internal/usertext"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var telegramManager = notify.NewTelegramManager("reports", nil)

func runAIEvaluation() error {
	result, err := aieval.Run(aieval.Config{})
	if err != nil {
		return err
	}
	fmt.Println(result.Summary)
	return nil
}

func runAIWatch(ctx context.Context, cfg config.Config, db *storage.DB) error {
	if err := fetch(ctx, cfg, db); err != nil {
		return err
	}
	analysis, err := analyze(ctx, cfg, db)
	if err != nil {
		return err
	}
	p, err := plan(ctx, cfg, db)
	if err != nil {
		return err
	}
	status, _ := formatStatus(cfg, db)
	snap := aiagent.Snapshot{Analysis: analysis, Plan: p, Status: status}
	var caller aiagent.JSONCaller
	if cfg.AI.Enabled {
		client, err := newObservedLLMClient(cfg, db, "ai_watch", "command", "ai_watch", "", 0)
		if err != nil {
			log.Printf("ai warning: %v", err)
		} else {
			caller = client
		}
	}
	report, err := aiagent.Generate(ctx, caller, snap)
	if err != nil {
		log.Printf("ai report warning: %v", err)
	}
	if err := db.SaveReport("ai_watch", report.TelegramText); err != nil {
		return err
	}
	if err := saveJSONFile("reports", "ai_watch_latest.json", report); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "ai_watch_latest.md"), []byte(report.TelegramText), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && (!cfg.AI.Enabled || cfg.AI.TelegramEnabled) {
		sendScheduledTelegram(ctx, cfg, "run-ai-watch", report.TelegramText)
	}
	fmt.Println(report.TelegramText)
	return nil
}

func runResearchDoctor(ctx context.Context, cfg config.Config) (research.DoctorResult, error) {
	result := research.RunDoctor(ctx, cfg)
	if err := saveJSONFile("reports", "research_doctor_latest.json", result); err != nil {
		return result, err
	}
	md := research.DoctorMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join("reports", "research_doctor_latest.md"), []byte(md), 0600); err != nil {
		return result, err
	}
	fmt.Println(md)
	return result, nil
}

func runResearchBrief(ctx context.Context, cfg config.Config, notifyTelegram bool) (research.BriefResult, error) {
	return runResearchBriefWithDB(ctx, cfg, nil, notifyTelegram)
}

func runResearchBriefWithDB(ctx context.Context, cfg config.Config, db *storage.DB, notifyTelegram bool) (research.BriefResult, error) {
	result := research.BuildBrief(ctx, cfg)
	if err := saveJSONFile("reports", "research_brief_latest.json", result); err != nil {
		return result, err
	}
	md := research.BriefMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join("reports", "research_brief_latest.md"), []byte(md), 0600); err != nil {
		return result, err
	}
	const label = "research-brief"
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && cfg.AI.TelegramEnabled && notifyTelegram && shouldAutoSendTelegram(label) {
		telegramText := buildResearchTelegramText(ctx, cfg, db, result)
		sendScheduledTelegram(ctx, cfg, label, telegramText)
	}
	fmt.Println(md)
	return result, nil
}

// buildResearchTelegramText tries AI analysis first; falls back to deterministic formatter.

func buildResearchTelegramText(ctx context.Context, cfg config.Config, db *storage.DB, result research.BriefResult) string {
	if cfg.AI.Enabled && len(result.Items) > 0 {
		llmClient, err := newObservedLLMClient(cfg, db, "research_brief", "research", "telegram_format", "", 0)
		if err != nil {
			log.Printf("research ai client: %v — using deterministic formatter", err)
		} else {
			aiText, err := research.AnalyzeBriefWithAI(ctx, llmClient, result)
			if err != nil {
				log.Printf("research ai analysis: %v — using deterministic formatter", err)
			} else if aiText != "" {
				log.Printf("research brief: AI analysis ok (%d chars)", len(aiText))
				return aiText
			}
		}
	}
	return telegramreport.ResearchBriefHumanText(result)
}

func shouldAutoSendTelegram(label string) bool {
	switch label {
	case "hermes-opening", "hermes-midday", "hermes-closing", "hermes-digest", "hermes-decision", "hermes-execution", "hermes-exit", "hermes-safety", "expert-report", "market-critical", "market-watch-error", "scheduler-heartbeat-stale", "operator-halt", "operator-resume", "reconcile-live-orders", "auto-live-management", "manual-live-order", "cancel-all-live-orders":
		return true
	default:
		return false
	}
}

func sendScheduledTelegram(ctx context.Context, cfg config.Config, label, text string) {
	if !shouldAutoSendTelegram(label) {
		log.Printf("telegram scheduled notification suppressed [%s]", label)
		return
	}
	sendTelegram(ctx, cfg, label, text)
}

func sendTelegram(ctx context.Context, cfg config.Config, label, text string) {
	token := os.Getenv("TELEGRAM_TOKEN")
	chatID := firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID"))
	text = usertext.TelegramVietnamese(text)
	result, err := telegramManager.Send(ctx, token, chatID, label, text)
	if err != nil {
		if errors.Is(err, notify.ErrTelegramConfig) {
			log.Printf("telegram config warning [%s]: %v", label, err)
			return
		}
		log.Printf("telegram warning [%s]: %v", label, err)
		return
	}
	log.Printf("telegram sent ok [%s] msg_id=%d", label, result.MessageID)
}
