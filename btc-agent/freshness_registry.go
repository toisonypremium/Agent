package main

import "time"

type FreshnessPolicy struct {
	Name         string
	MaxAge       time.Duration
	Critical     bool
	BlocksBuy    bool
	BlocksReduce bool
}

var freshnessRegistry = map[string]FreshnessPolicy{
	"operations_plan_latest.json":        {Name: "operations_plan", MaxAge: 35 * time.Minute, Critical: true, BlocksBuy: true},
	"live_doctor_latest.json":            {Name: "live_doctor", MaxAge: 35 * time.Minute, Critical: true, BlocksBuy: true, BlocksReduce: true},
	"live_auto_audit_latest.json":        {Name: "live_audit", MaxAge: 90 * time.Minute, Critical: true, BlocksBuy: true},
	"scheduler_heartbeat_latest.json":    {Name: "scheduler_heartbeat", MaxAge: 10 * time.Minute, Critical: true, BlocksBuy: true},
	"live_reconcile_latest.json":         {Name: "live_reconcile", MaxAge: 35 * time.Minute, Critical: true, BlocksBuy: true, BlocksReduce: true},
	"live_readiness_latest.json":         {Name: "live_readiness", MaxAge: 35 * time.Minute, Critical: true, BlocksBuy: true},
	"hermes_shadow_decision_latest.json": {Name: "hermes_shadow", MaxAge: 35 * time.Minute, Critical: true, BlocksBuy: true},
	"hermes_report_latest.json":          {Name: "hermes_narrative", MaxAge: 90 * time.Minute, Critical: false},
	"capital_plan_research_latest.json":  {Name: "capital_research", MaxAge: 90 * time.Minute, Critical: false},
}

func freshnessPolicy(name string, fallback time.Duration) FreshnessPolicy {
	if policy, ok := freshnessRegistry[name]; ok {
		return policy
	}
	return FreshnessPolicy{Name: name, MaxAge: fallback}
}
