package main

import (
	"log"

	"btc-agent/internal/config"
	"btc-agent/internal/llm"
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
		}
	}
	return llm.NewFromEnvConfig(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, llm.Config{
		Model: cfg.AI.Model, MaxTokens: maxTokens, Temp: cfg.AI.Temperature,
		Purpose: purpose, TriggerSource: triggerSource, TriggerReason: triggerReason,
		StateHash: stateHash, Observer: observer,
	})
}
