package storage

import (
	"context"

	"btc-agent/internal/runtime/outbox"
)

type OutboxHealth struct {
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Delivered  int `json:"delivered"`
	DeadLetter int `json:"dead_letter"`
}

func (d *DB) OutboxHealth() (OutboxHealth, error) {
	out := OutboxHealth{}
	rows, err := d.Query(`SELECT status,COUNT(*) FROM outbox_events GROUP BY status`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return out, err
		}
		switch status {
		case outbox.StatusPending:
			out.Pending = count
		case outbox.StatusProcessing:
			out.Processing = count
		case outbox.StatusDelivered:
			out.Delivered = count
		case outbox.StatusDeadLetter:
			out.DeadLetter = count
		}
	}
	return out, rows.Err()
}

func (d *DB) ExecutionLeaseForDashboard(ctx context.Context, name string) (map[string]any, error) {
	lease, ok, err := d.CurrentExecutionLease(ctx, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return map[string]any{"present": false}, nil
	}
	return map[string]any{"present": true, "name": lease.Name, "instance_id": lease.InstanceID, "fencing_token": lease.FencingToken, "acquired_at": lease.AcquiredAt, "expires_at": lease.ExpiresAt}, nil
}
