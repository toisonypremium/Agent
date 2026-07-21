package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/llm"
	"btc-agent/internal/runtime/outbox"
	"btc-agent/internal/storage"
)

func newObservedLLMClient(cfg config.Config, db *storage.DB, purpose, triggerSource, triggerReason, stateHash string, maxTokens int) (*llm.Client, error) {
	if maxTokens <= 0 {
		maxTokens = config.EffectiveAIMaxTokens(cfg, purpose)
	}
	observer := func(result llm.CallResult) {
		if db == nil {
			return
		}
		event := storage.LLMUsageEvent{
			RequestID: result.RequestID, Timestamp: result.Timestamp, Purpose: purpose,
			TriggerSource: triggerSource, TriggerReason: triggerReason, Model: result.Model,
			PromptTokens: result.Usage.PromptTokens, CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens: result.Usage.TotalTokens, UsageAvailable: result.Usage.Available,
			LatencyMS: result.LatencyMS, Status: result.Status, ErrorClass: result.ErrorClass,
			StateHash: stateHash,
		}
		if err := db.SaveLLMUsageEvent(event); err != nil {
			log.Printf("[LLM_USAGE] persist failed purpose=%s class=storage", purpose)
			return
		}
		enqueueLLMUsageCloud(context.Background(), db, event)
	}
	return llm.NewFromEnvConfig(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, llm.Config{
		Model: cfg.AI.Model, MaxTokens: maxTokens, Temp: cfg.AI.Temperature,
		Purpose: purpose, TriggerSource: triggerSource, TriggerReason: triggerReason,
		StateHash: stateHash, Observer: observer,
	})
}

func enqueueLLMUsageCloud(ctx context.Context, db *storage.DB, event storage.LLMUsageEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	for _, destination := range configuredCloudDestinations() {
		item := outbox.Item{
			ID:        "llm-usage-" + destination + "-" + event.RequestID,
			EventType: "llm_usage", Destination: destination, Payload: payload,
			IdempotencyKey: destination + ":llm_usage:" + event.RequestID,
			CreatedAt:      event.Timestamp,
		}
		if err := db.EnqueueOutbox(ctx, item); err != nil && !strings.Contains(strings.ToLower(err.Error()), "unique") {
			log.Printf("[LLM_USAGE] cloud enqueue failed purpose=%s destination=%s class=storage", event.Purpose, destination)
		}
	}
}

func configuredCloudDestinations() []string {
	out := []string{}
	if strings.TrimSpace(os.Getenv("SUPABASE_URL")) != "" && strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY")) != "" {
		out = append(out, "supabase")
	}
	if strings.TrimSpace(os.Getenv("R2_PRESIGNED_PUT_URL")) != "" || (strings.TrimSpace(os.Getenv("R2_ENDPOINT")) != "" && strings.TrimSpace(os.Getenv("R2_BUCKET")) != "" && strings.TrimSpace(os.Getenv("R2_ACCESS_KEY_ID")) != "" && strings.TrimSpace(os.Getenv("R2_SECRET_ACCESS_KEY")) != "") {
		out = append(out, "r2")
	}
	return out
}
