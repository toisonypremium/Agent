package webconsole

import (
	"fmt"
	"strings"
	"time"
)

type AuditEvent struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Result    string `json:"result"`
	RequestID string `json:"request_id"`
}
type AuditPage struct {
	Events []AuditEvent `json:"events"`
	Limit  int          `json:"limit"`
}

// Audit exposes fixed metadata columns only; payload_json and free-text reasons
// remain server-side because they may contain operator-sensitive details.
func (s *Service) Audit(limit int, actorRole string) (AuditPage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id,timestamp,identity,action,result,request_id FROM operator_audit_events ORDER BY timestamp DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return AuditPage{}, fmt.Errorf("read operator audit: %w", err)
	}
	defer rows.Close()
	out := AuditPage{Events: make([]AuditEvent, 0, limit), Limit: limit}
	for rows.Next() {
		var e AuditEvent
		var ts int64
		var actor string
		if err := rows.Scan(&e.ID, &ts, &actor, &e.Action, &e.Result, &e.RequestID); err != nil {
			return AuditPage{}, err
		}
		e.Timestamp = time.Unix(ts, 0).UTC().Format(time.RFC3339)
		e.Actor = maskedActor(actor, actorRole)
		out.Events = append(out.Events, e)
	}
	return out, rows.Err()
}
func maskedActor(identity, role string) string {
	identity = strings.TrimSpace(identity)
	if role == "auditor" || role == RoleOperator {
		return identity
	}
	if identity == "" {
		return ""
	}
	return "actor-" + shortHash(identity)
}
func shortHash(value string) string {
	var h uint32 = 2166136261
	for _, r := range value {
		h ^= uint32(r)
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}
