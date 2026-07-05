package liveguard

import "testing"

func TestSupervisorRefreshSummaryWarnsOnReasons(t *testing.T) {
	result := SupervisorResult{Action: SupervisorActionHeartbeat, Reasons: []string{"one", "one"}}
	result.RefreshSummary()
	if result.Status != SupervisorWarn {
		t.Fatalf("status=%s want %s", result.Status, SupervisorWarn)
	}
	if len(result.Reasons) != 1 {
		t.Fatalf("reasons=%v want unique", result.Reasons)
	}
	if result.Summary == "" {
		t.Fatal("expected summary")
	}
}

func TestSupervisorRefreshSummaryAutoHalted(t *testing.T) {
	result := SupervisorResult{AutoHalted: true, ConsecutiveErrors: 3}
	result.RefreshSummary()
	if result.Status != SupervisorHalted {
		t.Fatalf("status=%s want %s", result.Status, SupervisorHalted)
	}
	if result.Action != SupervisorActionHeartbeat {
		t.Fatalf("action=%s want default heartbeat", result.Action)
	}
}

func TestSupervisorRefreshSummaryWarnsOnManagedBlocked(t *testing.T) {
	result := SupervisorResult{Managed: &ManagedCycleResult{Status: ManagedCycleBlocked, Reasons: []string{"order placer/canceler unavailable"}}}
	result.RefreshSummary()
	if result.Status != SupervisorWarn {
		t.Fatalf("status=%s want %s", result.Status, SupervisorWarn)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != "order placer/canceler unavailable" {
		t.Fatalf("unexpected reasons: %v", result.Reasons)
	}
}

func TestSupervisorRefreshSummaryKeepsDryRunNoActionOK(t *testing.T) {
	result := SupervisorResult{Managed: &ManagedCycleResult{Status: ManagedCycleDryRun}}
	result.RefreshSummary()
	if result.Status != SupervisorOK {
		t.Fatalf("status=%s want %s", result.Status, SupervisorOK)
	}
}
