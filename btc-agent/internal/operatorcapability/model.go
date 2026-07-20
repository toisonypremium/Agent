package operatorcapability

import "strings"

type Capability string

const (
	Halt                Capability = "HALT"
	Resume              Capability = "RESUME"
	SetHermesObserve    Capability = "SET_HERMES_OBSERVE"
	SetHermesShadow     Capability = "SET_HERMES_SHADOW"
	SetHermesCanary     Capability = "SET_HERMES_CANARY"
	ReduceRiskCaps      Capability = "REDUCE_RISK_CAPS"
	ProposeRiskIncrease Capability = "PROPOSE_RISK_INCREASE"
	ApplyApprovedChange Capability = "APPLY_APPROVED_CHANGE"
	ToggleCircuitTimer  Capability = "TOGGLE_CIRCUIT_TIMER"
)

type Flags struct{ Halt, Resume, HermesMode, ReduceRisk, IncreaseRisk, CircuitTimer bool }
type State struct {
	Authenticated                                                                                     bool
	Identity                                                                                          string
	SecondApproverConfigured, Halted, HeartbeatFresh, DoctorHealthy, ReconcileClean, CooldownComplete bool
	Flags                                                                                             Flags
}
type Decision struct {
	Capabilities []Capability          `json:"capabilities"`
	Denied       map[Capability]string `json:"denied"`
}

func Evaluate(s State) Decision {
	d := Decision{Denied: map[Capability]string{}}
	all := []Capability{Halt, Resume, SetHermesObserve, SetHermesShadow, SetHermesCanary, ReduceRiskCaps, ProposeRiskIncrease, ApplyApprovedChange, ToggleCircuitTimer}
	if !s.Authenticated || strings.TrimSpace(s.Identity) == "" {
		for _, c := range all {
			d.Denied[c] = "VERIFIED_IDENTITY_REQUIRED"
		}
		return d
	}
	allow := func(c Capability, ok bool, why string) {
		if ok {
			d.Capabilities = append(d.Capabilities, c)
		} else {
			d.Denied[c] = why
		}
	}
	allow(Halt, s.Flags.Halt, "FEATURE_DISABLED")
	fresh := s.HeartbeatFresh && s.DoctorHealthy && s.ReconcileClean
	allow(Resume, s.Flags.Resume && s.Halted && fresh && s.CooldownComplete, resumeReason(s, fresh))
	allow(SetHermesObserve, s.Flags.HermesMode, "FEATURE_DISABLED")
	allow(SetHermesShadow, s.Flags.HermesMode && fresh, modeReason(s.Flags.HermesMode, fresh))
	allow(SetHermesCanary, s.Flags.HermesMode && fresh, modeReason(s.Flags.HermesMode, fresh))
	allow(ReduceRiskCaps, s.Flags.ReduceRisk, "FEATURE_DISABLED")
	inc := s.Flags.IncreaseRisk && fresh && s.SecondApproverConfigured
	allow(ProposeRiskIncrease, inc, increaseReason(s, fresh))
	allow(ApplyApprovedChange, inc, increaseReason(s, fresh))
	allow(ToggleCircuitTimer, s.Flags.CircuitTimer, "FEATURE_DISABLED")
	return d
}
func resumeReason(s State, fresh bool) string {
	if !s.Flags.Resume {
		return "FEATURE_DISABLED"
	}
	if !s.Halted {
		return "SYSTEM_NOT_HALTED"
	}
	if !fresh {
		return "CRITICAL_SAFETY_EVIDENCE_NOT_READY"
	}
	if !s.CooldownComplete {
		return "COOLDOWN_ACTIVE"
	}
	return "DENIED"
}
func modeReason(enabled, fresh bool) string {
	if !enabled {
		return "FEATURE_DISABLED"
	}
	if !fresh {
		return "CRITICAL_SAFETY_EVIDENCE_NOT_READY"
	}
	return "DENIED"
}
func increaseReason(s State, fresh bool) string {
	if !s.Flags.IncreaseRisk {
		return "FEATURE_DISABLED"
	}
	if !fresh {
		return "CRITICAL_SAFETY_EVIDENCE_NOT_READY"
	}
	if !s.SecondApproverConfigured {
		return "SECOND_APPROVER_REQUIRED"
	}
	return "DENIED"
}
