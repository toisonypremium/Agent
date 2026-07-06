package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/textsafe"
)

// AIBriefCaller is satisfied by *llm.Client (ChatJSON).
type AIBriefCaller interface {
	ChatText(ctx context.Context, prompt string) (string, error)
}

// AIBriefAnalysis is the structured output from the AI model.
type AIBriefAnalysis struct {
	MarketSentiment string   `json:"market_sentiment"` // BULLISH / BEARISH / NEUTRAL / MIXED
	KeyRisks        []string `json:"key_risks"`
	KeyOpportunity  []string `json:"key_opportunity"`
	BTCImpact       string   `json:"btc_impact"`     // SHORT summary ≤80 chars
	AltcoinImpact   string   `json:"altcoin_impact"` // SHORT summary ≤80 chars
	ActionBias      string   `json:"action_bias"`    // HOLD / ACCUMULATE_ON_DIP / REDUCE / WATCH
	TelegramText    string   `json:"telegram_text"`  // formatted Telegram message ≤1800 chars
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
		Risk    string `json:"risk"`
		Tags    string `json:"tags"`
		Summary string `json:"summary,omitempty"`
	}
	items := []compactItem{}
	limit := len(result.Items)
	if limit > 10 {
		limit = 10
	}
	for _, it := range result.Items[:limit] {
		items = append(items, compactItem{
			Source:  it.Source,
			Title:   it.Title,
			Risk:    it.Risk,
			Tags:    strings.Join(it.Tags, ","),
			Summary: it.Summary,
		})
	}
	payload, _ := json.MarshalIndent(items, "", "  ")

	now := time.Now().UTC().Format("02/01 15:04 UTC")
	prompt := fmt.Sprintf(`You are an institutional crypto trading desk analyst for a spot-only BTC/altcoin accumulation bot (NO futures, NO leverage, NO market orders).
Analyze these %d news/RSS items collected at %s. News is EVIDENCE ONLY; do not dump links or raw headlines unless needed.
Return ONLY the Telegram-ready Vietnamese text, no JSON, no markdown fences.
Expert report style:
- Start with one clear market stance: Risk-on / Neutral / Risk-off / Mixed.
- Explain WHY in 2-3 sentences: flows, macro/policy, exchange risk, ETF/institutional demand, derivatives/options tone, on-chain/whale accumulation if mentioned.
- Separate BTC impact from altcoin impact (ETH/SOL/RENDER if present).
- Map risk vs opportunity: what can improve setup, what can invalidate it.
- Provide action bias for this bot: WAIT / WATCH / ACCUMULATE_ON_DIP / HOLD_CASH. Must stay under deterministic Agent 1/2 authority.
Rules:
- ONLY spot BUY limit post-only. Never recommend futures, shorting, leverage, or market orders.
- No URLs in telegram_text. Do not paste source links. Mention evidence as "Tin nền:" short bullet titles only if useful.
- Be concise. telegram_text max 1900 chars, written in Vietnamese, expert tone, no hype.
- telegram_text must have sections: "Kết luận", "Luận điểm", "Rủi ro", "Cơ hội", "Kế hoạch bot".
- telegram_text must end with: "Research-only: không đặt lệnh, không override Agent 1/2."
- Never include API keys, secrets, or credential values.

News items:
%s`, limit, now, string(payload))

	text, err := caller.ChatText(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("ai brief analysis: %w", err)
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(text, "```"), "```"))

	// Safety: ensure required footer present
	if !strings.Contains(text, "Research-only") {
		text += "\nResearch-only: không đặt lệnh, không override Agent 1/2."
	}

	// Safety: never expose secret-like strings or raw URLs in Telegram.
	if textsafe.ContainsSecretLike(text) {
		return "", fmt.Errorf("ai brief output failed safety check")
	}
	text = textsafe.StripURLs(text)

	return text, nil
}
