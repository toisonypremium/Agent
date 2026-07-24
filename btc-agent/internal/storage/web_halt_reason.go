package storage

import "strings"

const (
	WebHaltReasonRuntimeIntegrity = "runtime_integrity"
	WebHaltReasonExchangeState    = "exchange_state"
	WebHaltReasonCapitalIntegrity = "capital_integrity"
	WebHaltReasonSecurityIncident = "security_incident"
)

func NormalizeWebHaltReason(value string) (string, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case WebHaltReasonRuntimeIntegrity, WebHaltReasonExchangeState, WebHaltReasonCapitalIntegrity, WebHaltReasonSecurityIncident:
		return value, true
	default:
		return "", false
	}
}
