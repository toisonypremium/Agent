package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AIBriefCaller is satisfied by *llm.Client (ChatJSON).
type AIBriefCaller interface {
	ChatJSON(ctx context.Context, prompt string, out any) error
}

// AIBriefAnalysis is the structured output from the AI model.
type AIBriefAnalysis struct {
	MarketSentiment string   `json:"market_sentiment"` // BULLISH / BEARISH / NEUTRAL / MIXED
	KeyRisks        []string `json:"key_risks"`
	KeyOpportunity  []string `json:"key_opportunity"`
	BTCImpact       string   `json:"btc_impact"`   // SHORT summary ≤80 chars
	AltcoinImpact   string   `json:"altcoin_impact"` // SHORT summary ≤80 chars
	ActionBias      string   `json:"action_bias"`  // HOLD / ACCUMULATE_ON_DIP / REDUCE / WATCH
	TelegramText    string   `json:"telegram_text"` // formatted Telegram message ≤1800 chars
}

// AnalyzeBriefWithAI sends RSS items to the LLM for strategy-level analysis.
// Returns formatted Telegram text. Falls back to empty string on error (caller uses raw format).
func AnalyzeBriefWithAI(ctx context.Context, caller AIBriefCaller, result BriefResult) (string, error) {
	if caller == nil || len(result.Items) == 0 {
		return "", nil
	}

	// Build compact item list for prompt (max 15 items to stay under token limits)
	type compactItem struct {
		Source  string `json:"source"`
		Title   string `json:"title"`
		URL     string `json:"url"`
		Risk    string `json:"risk"`
		Tags    string `json:"tags"`
		Summary string `json:"summary,omitempty"`
	}
	items := []compactItem{}
	limit := len(result.Items)
	if limit > 15 {
		limit = 15
	}
	for _, it := range result.Items[:limit] {
		items = append(items, compactItem{
			Source:  it.Source,
			Title:   it.Title,
			URL:     it.URL,
			Risk:    it.Risk,
			Tags:    strings.Join(it.Tags, ","),
			Summary: it.Summary,
		})
	}
	payload, _ := json.MarshalIndent(items, "", "  ")

	now := time.Now().UTC().Format("02/01 15:04 UTC")
	prompt := fmt.Sprintf(`Return exactly one valid JSON object. No markdown, no explanation outside JSON.
You are an expert crypto trading analyst for a spot-only BTC/altcoin accumulation bot (NO futures, NO leverage, NO market orders).
Analyze these %d news/RSS items collected at %s and produce a brief strategy-aware assessment.
Rules:
- ONLY spot BUY limit post-only. Never recommend futures, shorting, leverage, or market orders.
- Focus on BTC regime impact and altcoin (ETH/SOL/RENDER) opportunity/risk.
- Be concise. telegram_text max 1800 chars, written in Vietnamese.
- telegram_text must contain: sentiment header, 1-2 key risk lines, 1-2 opportunity lines, action_bias, and top 3-5 news items with URL.
- telegram_text must end with: "Research-only: không đặt lệnh, không override Agent 1/2."
- Never include API keys, secrets, or credential values.

JSON schema:
{
  "market_sentiment": "BULLISH|BEARISH|NEUTRAL|MIXED",
  "key_risks": ["..."],
  "key_opportunity": ["..."],
  "btc_impact": "short string ≤80 chars",
  "altcoin_impact": "short string ≤80 chars",
  "action_bias": "HOLD|ACCUMULATE_ON_DIP|REDUCE|WATCH",
  "telegram_text": "full Vietnamese Telegram message"
}

News items:
%s`, limit, now, string(payload))

	var analysis AIBriefAnalysis
	if err := caller.ChatJSON(ctx, prompt, &analysis); err != nil {
		return "", fmt.Errorf("ai brief analysis: %w", err)
	}

	// Safety: ensure required footer present
	if !strings.Contains(analysis.TelegramText, "Research-only") {
		analysis.TelegramText += "\nResearch-only: không đặt lệnh, không override Agent 1/2."
	}

	// Safety: never expose secret-like strings
	if containsSecretLike(analysis.TelegramText) {
		return "", fmt.Errorf("ai brief output failed safety check")
	}

	return analysis.TelegramText, nil
}

func containsSecretLike(s string) bool {
	lower := strings.ToLower(s)
	for _, pat := range []string{"api_key", "api_secret", "passphrase value", "telegram_token", "bearer "} {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}
