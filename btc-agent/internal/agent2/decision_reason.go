package agent2

import "btc-agent/internal/decision"

type ReasonCode = decision.ReasonCode

type ReasonSeverity = decision.ReasonSeverity

type ReasonScope = decision.ReasonScope

type DecisionReason = decision.DecisionReason

const (
	ReasonHardBlock = decision.SeverityHardBlock
	ReasonSoftWait  = decision.SeveritySoftWait
	ReasonInfo      = decision.SeverityInfo
)

const (
	ReasonScopeBTC       = decision.ScopeBTC
	ReasonScopeAsset     = decision.ScopeAsset
	ReasonScopeData      = decision.ScopeData
	ReasonScopeRisk      = decision.ScopeRisk
	ReasonScopeFlow      = decision.ScopeFlow
	ReasonScopeRotation  = decision.ScopeRotation
	ReasonScopeExecution = decision.ScopeExecution
)

const (
	ReasonBTCPermission    = decision.CodeBTCPermission
	ReasonBTCPanic         = decision.CodeBTCPanic
	ReasonBTCDowntrend     = decision.CodeBTCDowntrend
	ReasonFallingKnife     = decision.CodeFallingKnife
	ReasonFOMO             = decision.CodeFOMO
	ReasonRelativeStrength = decision.CodeRelativeStrength
	ReasonRotationScore    = decision.CodeRotationScore
	ReasonRotationRank     = decision.CodeRotationRank
	ReasonMMAccumulation   = decision.CodeMMAccumulation
	ReasonAssetFlowEntry   = decision.CodeAssetFlowEntry
	ReasonLiquidityQuality = decision.CodeLiquidityQuality
	ReasonDiscountZone     = decision.CodeDiscountZone
	ReasonRewardRisk       = decision.CodeRewardRisk
	ReasonDataWait         = decision.CodeDataWait
	ReasonExecutionLayer   = decision.CodeExecutionLayer
)

func NewDecisionReason(code ReasonCode, severity ReasonSeverity, scope ReasonScope, message string) DecisionReason {
	return decision.NewReason(code, severity, scope, message)
}

func AddReason(reasons []DecisionReason, reason DecisionReason) []DecisionReason {
	return decision.AddReason(reasons, reason)
}

func HasHardBlock(reasons []DecisionReason) bool {
	return decision.HasHardBlock(reasons)
}

func HasSoftWait(reasons []DecisionReason) bool {
	return decision.HasSoftWait(reasons)
}

func ReasonCodes(reasons []DecisionReason) []string {
	return decision.ReasonCodes(reasons)
}

func ReasonsBySeverity(reasons []DecisionReason, severity ReasonSeverity) []DecisionReason {
	return decision.ReasonsBySeverity(reasons, severity)
}

func PrimaryReason(reasons []DecisionReason) string {
	return decision.PrimaryReason(reasons)
}

func ReasonMessages(reasons []DecisionReason) []string {
	return decision.ReasonMessages(reasons)
}
