package storage

import (
	"database/sql"
	"time"
)

type ControlPlaneProposal struct {
	DecisionID       string    `json:"decision_id"`
	Caller           string    `json:"caller"`
	ReceivedAt       time.Time `json:"received_at"`
	PayloadSHA256    string    `json:"payload_sha256"`
	PayloadJSON      string    `json:"payload_json,omitempty"`
	SchemaVerdict    string    `json:"schema_verdict"`
	PolicyVerdict    string    `json:"policy_verdict"`
	ExecutionVerdict string    `json:"execution_verdict"`
	ReasonsJSON      string    `json:"reasons_json"`
}

func (d *DB) SaveControlPlaneProposal(p ControlPlaneProposal) (bool, error) {
	if p.ReceivedAt.IsZero() {
		p.ReceivedAt = time.Now().UTC()
	}
	res, err := d.Exec(`INSERT OR IGNORE INTO control_plane_proposals(decision_id,caller,received_at,payload_sha256,payload_json,schema_verdict,policy_verdict,execution_verdict,reasons_json) VALUES(?,?,?,?,?,?,?,?,?)`, p.DecisionID, p.Caller, p.ReceivedAt.Unix(), p.PayloadSHA256, p.PayloadJSON, p.SchemaVerdict, p.PolicyVerdict, p.ExecutionVerdict, p.ReasonsJSON)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (d *DB) ControlPlaneProposal(decisionID string) (ControlPlaneProposal, error) {
	var p ControlPlaneProposal
	var ts int64
	err := d.QueryRow(`SELECT decision_id,caller,received_at,payload_sha256,payload_json,schema_verdict,policy_verdict,execution_verdict,reasons_json FROM control_plane_proposals WHERE decision_id=?`, decisionID).Scan(&p.DecisionID, &p.Caller, &ts, &p.PayloadSHA256, &p.PayloadJSON, &p.SchemaVerdict, &p.PolicyVerdict, &p.ExecutionVerdict, &p.ReasonsJSON)
	if err != nil {
		return p, err
	}
	p.ReceivedAt = time.Unix(ts, 0).UTC()
	return p, nil
}

func (d *DB) RecentControlPlaneProposals(limit int) ([]ControlPlaneProposal, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := d.Query(`SELECT decision_id,caller,received_at,payload_sha256,'',schema_verdict,policy_verdict,execution_verdict,reasons_json FROM control_plane_proposals ORDER BY received_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ControlPlaneProposal{}
	for rows.Next() {
		var p ControlPlaneProposal
		var ts int64
		if err := rows.Scan(&p.DecisionID, &p.Caller, &ts, &p.PayloadSHA256, &p.PayloadJSON, &p.SchemaVerdict, &p.PolicyVerdict, &p.ExecutionVerdict, &p.ReasonsJSON); err != nil {
			return nil, err
		}
		p.ReceivedAt = time.Unix(ts, 0).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

var _ = sql.ErrNoRows
