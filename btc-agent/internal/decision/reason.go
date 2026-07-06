package decision

type ReasonCode string

type ReasonSeverity string

type ReasonScope string

const (
	SeverityHardBlock ReasonSeverity = "HARD_BLOCK"
	SeveritySoftWait  ReasonSeverity = "SOFT_WAIT"
	SeverityInfo      ReasonSeverity = "INFO"
)

const (
	ScopeBTC       ReasonScope = "BTC"
	ScopeAsset     ReasonScope = "ASSET"
	ScopeData      ReasonScope = "DATA"
	ScopeRisk      ReasonScope = "RISK"
	ScopeFlow      ReasonScope = "FLOW"
	ScopeRotation  ReasonScope = "ROTATION"
	ScopeExecution ReasonScope = "EXECUTION"
)

const (
	CodeBTCPermission    ReasonCode = "BTC_PERMISSION"
	CodeBTCPanic         ReasonCode = "BTC_PANIC"
	CodeBTCDowntrend     ReasonCode = "BTC_DOWNTREND"
	CodeFallingKnife     ReasonCode = "FALLING_KNIFE"
	CodeFOMO             ReasonCode = "FOMO"
	CodeRelativeStrength ReasonCode = "RELATIVE_STRENGTH"
	CodeRotationScore    ReasonCode = "ROTATION_SCORE"
	CodeRotationRank     ReasonCode = "ROTATION_RANK"
	CodeMMAccumulation   ReasonCode = "MM_ACCUMULATION"
	CodeAssetFlowEntry   ReasonCode = "ASSET_FLOW_ENTRY"
	CodeLiquidityQuality ReasonCode = "LIQUIDITY_QUALITY"
	CodeDiscountZone     ReasonCode = "DISCOUNT_ZONE"
	CodeRewardRisk       ReasonCode = "REWARD_RISK"
	CodeDataWait         ReasonCode = "DATA_WAIT"
	CodeExecutionLayer   ReasonCode = "EXECUTION_LAYER"
)

type DecisionReason struct {
	Code     ReasonCode     `json:"code"`
	Severity ReasonSeverity `json:"severity"`
	Scope    ReasonScope    `json:"scope"`
	Message  string         `json:"message"`
}

func NewReason(code ReasonCode, severity ReasonSeverity, scope ReasonScope, message string) DecisionReason {
	return DecisionReason{Code: code, Severity: severity, Scope: scope, Message: message}
}

func AddReason(reasons []DecisionReason, reason DecisionReason) []DecisionReason {
	if reason.Code == "" && reason.Message == "" {
		return reasons
	}
	if reason.Severity == "" {
		reason.Severity = SeverityInfo
	}
	return append(reasons, reason)
}

func HasHardBlock(reasons []DecisionReason) bool {
	for _, reason := range reasons {
		if reason.Severity == SeverityHardBlock {
			return true
		}
	}
	return false
}

func HasSoftWait(reasons []DecisionReason) bool {
	for _, reason := range reasons {
		if reason.Severity == SeveritySoftWait {
			return true
		}
	}
	return false
}

func ReasonCodes(reasons []DecisionReason) []string {
	out := []string{}
	seen := map[ReasonCode]bool{}
	for _, reason := range reasons {
		if reason.Code == "" || seen[reason.Code] {
			continue
		}
		seen[reason.Code] = true
		out = append(out, string(reason.Code))
	}
	return out
}

func ReasonsBySeverity(reasons []DecisionReason, severity ReasonSeverity) []DecisionReason {
	out := []DecisionReason{}
	for _, reason := range reasons {
		if reason.Severity == severity {
			out = append(out, reason)
		}
	}
	return out
}

func PrimaryReason(reasons []DecisionReason) string {
	for _, severity := range []ReasonSeverity{SeverityHardBlock, SeveritySoftWait, SeverityInfo} {
		for _, reason := range reasons {
			if reason.Severity == severity && reason.Message != "" {
				return reason.Message
			}
		}
	}
	return ""
}

func ReasonMessages(reasons []DecisionReason) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, reason := range reasons {
		if reason.Message == "" || seen[reason.Message] {
			continue
		}
		seen[reason.Message] = true
		out = append(out, reason.Message)
	}
	return out
}
