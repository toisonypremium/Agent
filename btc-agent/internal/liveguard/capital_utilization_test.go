package liveguard

import (
	"math"
	"testing"
)

func TestCapitalUtilizationRegimeTargets(t *testing.T) {
	down := EvaluateCapitalUtilization(CapitalUtilizationInput{TotalCapital: 1000, ExistingExposure: 100, ReserveCashRatio: .05, HardExposureCap: 700, MarketRegime: "DOWNTREND", AccumulationPhase: "MARKDOWN"})
	if math.Abs(down.TargetDeploymentUSDT-350) > .01 || math.Abs(down.AvailableDeploymentUSDT-250) > .01 || down.State != "UNDERUTILIZED" {
		t.Fatalf("bad markdown target %+v", down)
	}
	confirmed := EvaluateCapitalUtilization(CapitalUtilizationInput{TotalCapital: 1000, ExistingExposure: 100, ReserveCashRatio: .05, HardExposureCap: 700, MarketRegime: "ACCUMULATION", AccumulationPhase: "ACCUMULATION_CONFIRMED"})
	if confirmed.TargetDeploymentUSDT != 700 || confirmed.AvailableDeploymentUSDT != 600 {
		t.Fatalf("bad confirmed target %+v", confirmed)
	}
}
func TestCapitalUtilizationPreservesCashAndPanic(t *testing.T) {
	r := EvaluateCapitalUtilization(CapitalUtilizationInput{TotalCapital: 1000, ExistingExposure: 100, ReserveCashRatio: .90, HardExposureCap: 700, MarketRegime: "ACCUMULATION", AccumulationPhase: "ACCUMULATION_CONFIRMED"})
	if math.Abs(r.TargetDeploymentUSDT-100) > .01 || r.State != "TARGET_REACHED" {
		t.Fatalf("reserve not enforced %+v", r)
	}
	p := EvaluateCapitalUtilization(CapitalUtilizationInput{TotalCapital: 1000, ExistingExposure: 200, ReserveCashRatio: .05, HardExposureCap: 700, PanicSelling: true, MarketRegime: "PANIC_SELLING"})
	if p.AvailableDeploymentUSDT != 0 || p.State != "TARGET_REACHED" {
		t.Fatalf("panic should add no risk %+v", p)
	}
}
