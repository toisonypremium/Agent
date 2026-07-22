package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"btc-agent/internal/textsafe"
)

type ExpertAnalysis struct {
	Synthesis             string   `json:"synthesis"`
	BullCase              Scenario `json:"bull_case"`
	BaseCase              Scenario `json:"base_case"`
	BearCase              Scenario `json:"bear_case"`
	RiskAssessment        string   `json:"risk_assessment"`
	KeyCatalysts          []string `json:"key_catalysts"`
	ActionRecommendations []string `json:"action_recommendations"`
	ConfidenceLevel       string   `json:"confidence_level"`
	TelegramText          string   `json:"telegram_text"`
}

type Scenario struct {
	Probability float64 `json:"probability"`
	Conditions  string  `json:"conditions"`
}

type ExpertCaller interface {
	ChatText(ctx context.Context, prompt string) (string, error)
}

func AnalyzeExpertReportWithAI(ctx context.Context, caller ExpertCaller, report ExpertReport) (ExpertAnalysis, error) {
	if caller == nil {
		return ExpertAnalysis{}, nil
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return ExpertAnalysis{}, err
	}
	prompt := fmt.Sprintf(`You are a senior institutional crypto and macro analyst. Analyze ONLY the evidence in this report. Return exactly one valid JSON object, no markdown fences.

Rules:
- Separate facts from inference. Never invent a data point, event, source, or causal claim.
- Use the source confidence values: lower confidence means less certainty.
- StrategyContext is supplementary deterministic bot context, not external evidence; never cite it as a news fact.
- Explain macro/rates/liquidity, policy/regulation, trade/geopolitics, then BTC and altcoin implications.
- Give bull/base/bear scenarios. Each probability must be 0..1 and all three must sum to approximately 1.
- This is READ-ONLY analysis. Never place, cancel, or modify orders. Do not override deterministic Agent 1/2, risk gates, capital allocation, or safety controls.
- Recommendations must be monitoring/risk-management observations only, not execution instructions.
- Write TelegramText in Vietnamese, max 3600 chars, professional and concise. Include sections: Kết luận, Vĩ mô, BTC/Altcoin, Kịch bản, Rủi ro, Theo dõi.
- Do not include URLs, API keys, credentials, or unsupported price targets.

JSON schema:
{
  "synthesis":"...",
  "bull_case":{"probability":0.0,"conditions":"..."},
  "base_case":{"probability":0.0,"conditions":"..."},
  "bear_case":{"probability":0.0,"conditions":"..."},
  "risk_assessment":"...",
  "key_catalysts":["..."],
  "action_recommendations":["..."],
  "confidence_level":"HIGH|MEDIUM|LOW",
  "telegram_text":"..."
}

Evidence report:
%s`, string(payload))
	text, err := caller.ChatText(ctx, prompt)
	if err != nil {
		return ExpertAnalysis{}, err
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(text, "```"), "```"))
	var out ExpertAnalysis
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return ExpertAnalysis{}, fmt.Errorf("expert analysis json: %w", err)
	}
	if textsafe.ContainsSecretLike(out.TelegramText) {
		return ExpertAnalysis{}, fmt.Errorf("expert analysis contains secret-like text")
	}
	out.TelegramText = textsafe.StripURLs(out.TelegramText)
	out.ConfidenceLevel = normalizedConfidence(out.ConfidenceLevel)
	if out.TelegramText == "" {
		return ExpertAnalysis{}, fmt.Errorf("expert analysis telegram text required")
	}
	return out, nil
}

func normalizedConfidence(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "HIGH", "MEDIUM", "LOW":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "LOW"
	}
}
