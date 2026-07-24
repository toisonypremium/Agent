package webconsole

import "time"

type MarketPlannerView struct {
	Available        bool     `json:"available"`
	GeneratedAt      string   `json:"generated_at,omitempty"`
	PriceUSDT        float64  `json:"price_usdt,omitempty"`
	Regime           string   `json:"regime,omitempty"`
	Permission       string   `json:"permission,omitempty"`
	PermissionReason string   `json:"permission_reason,omitempty"`
	RiskLevel        string   `json:"risk_level,omitempty"`
	FallingKnifeRisk string   `json:"falling_knife_risk,omitempty"`
	FOMORisk         string   `json:"fomo_risk,omitempty"`
	MarketSummary    string   `json:"market_summary,omitempty"`
	PlanState        string   `json:"plan_state,omitempty"`
	PlanSummary      string   `json:"plan_summary,omitempty"`
	Warnings         []string `json:"warnings"`
}

// MarketPlanner returns approved decision summaries only; raw strategy payloads,
// prompts, and executable order layers remain outside Web Console authority.
func (s *Service) MarketPlanner() (MarketPlannerView, error) {
	analysis, err := s.db.LatestAnalysis()
	if err != nil {
		return MarketPlannerView{Warnings: []string{}}, nil
	}
	out := MarketPlannerView{Available: true, GeneratedAt: analysis.Timestamp.UTC().Format(time.RFC3339), PriceUSDT: analysis.BTCPrice, Regime: analysis.MarketRegime, Permission: string(analysis.ActionPermission), PermissionReason: analysis.PermissionReason, RiskLevel: string(analysis.RiskLevel), FallingKnifeRisk: string(analysis.FallingKnifeRisk), FOMORisk: string(analysis.FomoRisk), MarketSummary: analysis.Summary, Warnings: []string{}}
	if plan, err := s.db.LatestPlan(); err == nil {
		out.PlanState = string(plan.State)
		out.PlanSummary = plan.Summary
		out.Warnings = append(out.Warnings, plan.Warnings...)
	}
	return out, nil
}
