package storage

import (
	"database/sql"
	"time"
)

type LLMUsageEvent struct {
	RequestID        string    `json:"request_id"`
	Timestamp        time.Time `json:"timestamp"`
	Purpose          string    `json:"purpose"`
	TriggerSource    string    `json:"trigger_source,omitempty"`
	TriggerReason    string    `json:"trigger_reason,omitempty"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	TotalTokens      int       `json:"total_tokens,omitempty"`
	UsageAvailable   bool      `json:"usage_available"`
	LatencyMS        int64     `json:"latency_ms"`
	Status           string    `json:"status"`
	ErrorClass       string    `json:"error_class,omitempty"`
	StateHash        string    `json:"state_hash,omitempty"`
}

type LLMUsageGroup struct {
	Key              string `json:"key"`
	Calls            int    `json:"calls"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

type LLMUsageSummary struct {
	From             time.Time       `json:"from"`
	To               time.Time       `json:"to"`
	Calls            int             `json:"calls"`
	SkippedCalls     int             `json:"skipped_calls"`
	FailedCalls      int             `json:"failed_calls"`
	UsageUnavailable int             `json:"usage_unavailable"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens"`
	RepeatedHashes   int             `json:"repeated_state_hashes"`
	ByPurpose        []LLMUsageGroup `json:"by_purpose"`
	ByModel          []LLMUsageGroup `json:"by_model"`
}

func (d *DB) SaveLLMUsageEvent(e LLMUsageEvent) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	var prompt, completion, total any
	if e.UsageAvailable {
		prompt, completion, total = e.PromptTokens, e.CompletionTokens, e.TotalTokens
	}
	_, err := d.Exec(`INSERT INTO llm_usage_events(request_id,timestamp,purpose,trigger_source,trigger_reason,model,prompt_tokens,completion_tokens,total_tokens,usage_available,latency_ms,status,error_class,state_hash) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.RequestID, e.Timestamp.Unix(), e.Purpose, e.TriggerSource, e.TriggerReason, e.Model, prompt, completion, total, boolInt(e.UsageAvailable), e.LatencyMS, e.Status, e.ErrorClass, e.StateHash)
	return err
}

func (d *DB) LLMUsageSummaryBetween(from, to time.Time) (LLMUsageSummary, error) {
	out := LLMUsageSummary{From: from.UTC(), To: to.UTC(), ByPurpose: []LLMUsageGroup{}, ByModel: []LLMUsageGroup{}}
	err := d.QueryRow(`SELECT COALESCE(SUM(CASE WHEN status<>'skipped' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status='skipped' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status='error' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status<>'skipped' AND usage_available=0 THEN 1 ELSE 0 END),0), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0), COALESCE(SUM(total_tokens),0) FROM llm_usage_events WHERE timestamp>=? AND timestamp<?`, from.Unix(), to.Unix()).Scan(&out.Calls, &out.SkippedCalls, &out.FailedCalls, &out.UsageUnavailable, &out.PromptTokens, &out.CompletionTokens, &out.TotalTokens)
	if err != nil {
		return out, err
	}
	if err = d.QueryRow(`SELECT COALESCE(SUM(n-1),0) FROM (SELECT COUNT(*) n FROM llm_usage_events WHERE timestamp>=? AND timestamp<? AND state_hash<>'' GROUP BY purpose,state_hash HAVING COUNT(*)>1)`, from.Unix(), to.Unix()).Scan(&out.RepeatedHashes); err != nil {
		return out, err
	}
	out.ByPurpose, err = d.llmUsageGroups(from, to, "purpose")
	if err != nil {
		return out, err
	}
	out.ByModel, err = d.llmUsageGroups(from, to, "model")
	return out, err
}

func (d *DB) llmUsageGroups(from, to time.Time, column string) ([]LLMUsageGroup, error) {
	if column != "purpose" && column != "model" {
		return nil, sql.ErrNoRows
	}
	rows, err := d.Query(`SELECT `+column+`,COUNT(*),COALESCE(SUM(prompt_tokens),0),COALESCE(SUM(completion_tokens),0),COALESCE(SUM(total_tokens),0) FROM llm_usage_events WHERE timestamp>=? AND timestamp<? GROUP BY `+column+` ORDER BY COUNT(*) DESC,`+column, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LLMUsageGroup{}
	for rows.Next() {
		var g LLMUsageGroup
		if err := rows.Scan(&g.Key, &g.Calls, &g.PromptTokens, &g.CompletionTokens, &g.TotalTokens); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
