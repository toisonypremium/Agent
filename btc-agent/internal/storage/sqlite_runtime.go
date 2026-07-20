package storage

import (
	"btc-agent/internal/runtime/outbox"
	"btc-agent/internal/runtime/ownership"
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AcquireExecutionLease atomically takes an expired/free lease and advances its
// fencing token. The transaction prevents two local database users becoming owner.
func (d *DB) AcquireExecutionLease(ctx context.Context, name, instanceID string, now time.Time, ttl time.Duration) (ownership.Lease, bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return ownership.Lease{}, false, err
	}
	defer tx.Rollback()
	var currentInstance string
	var fence, acquiredAt, expiresAt int64
	err = tx.QueryRowContext(ctx, `SELECT instance_id,fencing_token,acquired_at,expires_at FROM execution_leases WHERE name=?`, name).Scan(&currentInstance, &fence, &acquiredAt, &expiresAt)
	if err != nil && err != sql.ErrNoRows {
		return ownership.Lease{}, false, err
	}
	if err == nil && expiresAt > now.Unix() {
		if currentInstance != instanceID {
			return ownership.Lease{}, false, nil
		}
		lease := ownership.Lease{Name: name, InstanceID: currentInstance, FencingToken: fence, AcquiredAt: time.Unix(acquiredAt, 0).UTC(), ExpiresAt: time.Unix(expiresAt, 0).UTC()}
		if err = tx.Commit(); err != nil {
			return ownership.Lease{}, false, err
		}
		return lease, true, nil
	}
	fence++
	if fence < 1 {
		fence = 1
	}
	lease := ownership.Lease{Name: name, InstanceID: instanceID, FencingToken: fence, AcquiredAt: now, ExpiresAt: now.Add(ttl)}
	_, err = tx.ExecContext(ctx, `INSERT INTO execution_leases(name,instance_id,fencing_token,acquired_at,expires_at) VALUES(?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET instance_id=excluded.instance_id,fencing_token=excluded.fencing_token,acquired_at=excluded.acquired_at,expires_at=excluded.expires_at`, name, instanceID, fence, lease.AcquiredAt.Unix(), lease.ExpiresAt.Unix())
	if err != nil {
		return ownership.Lease{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return ownership.Lease{}, false, err
	}
	return lease, true, nil
}

func (d *DB) RenewExecutionLease(ctx context.Context, lease ownership.Lease, now time.Time, ttl time.Duration) (ownership.Lease, bool, error) {
	next := now.Add(ttl)
	res, err := d.ExecContext(ctx, `UPDATE execution_leases SET expires_at=? WHERE name=? AND instance_id=? AND fencing_token=? AND expires_at>?`, next.Unix(), lease.Name, lease.InstanceID, lease.FencingToken, now.Unix())
	if err != nil {
		return ownership.Lease{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ownership.Lease{}, false, err
	}
	if n != 1 {
		return ownership.Lease{}, false, nil
	}
	lease.ExpiresAt = next
	return lease, true, nil
}

func (d *DB) ReleaseExecutionLease(ctx context.Context, lease ownership.Lease) error {
	_, err := d.ExecContext(ctx, `DELETE FROM execution_leases WHERE name=? AND instance_id=? AND fencing_token=?`, lease.Name, lease.InstanceID, lease.FencingToken)
	return err
}

func (d *DB) CurrentExecutionLease(ctx context.Context, name string) (ownership.Lease, bool, error) {
	var l ownership.Lease
	var acquired, expires int64
	err := d.QueryRowContext(ctx, `SELECT name,instance_id,fencing_token,acquired_at,expires_at FROM execution_leases WHERE name=?`, name).Scan(&l.Name, &l.InstanceID, &l.FencingToken, &acquired, &expires)
	if err == sql.ErrNoRows {
		return ownership.Lease{}, false, nil
	}
	if err != nil {
		return ownership.Lease{}, false, err
	}
	l.AcquiredAt = time.Unix(acquired, 0).UTC()
	l.ExpiresAt = time.Unix(expires, 0).UTC()
	return l, true, nil
}

func (d *DB) EnqueueOutbox(ctx context.Context, item outbox.Item) error {
	if item.ID == "" || item.EventType == "" || item.Destination == "" || item.IdempotencyKey == "" {
		return fmt.Errorf("outbox id, type, destination and idempotency key required")
	}
	now := item.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	next := item.NextRetryAt
	if next.IsZero() {
		next = now
	}
	status := item.Status
	if status == "" {
		status = outbox.StatusPending
	}
	_, err := d.ExecContext(ctx, `INSERT INTO outbox_events(id,event_type,destination,payload,idempotency_key,status,retry_count,next_retry_at,last_error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.EventType, item.Destination, item.Payload, item.IdempotencyKey, status, item.RetryCount, next.Unix(), item.LastError, now.Unix(), now.Unix())
	return err
}

func (d *DB) ClaimOutbox(ctx context.Context, now time.Time, limit int) ([]outbox.Item, error) {
	if limit < 1 {
		return []outbox.Item{}, nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,event_type,destination,payload,idempotency_key,status,retry_count,next_retry_at,last_error,created_at FROM outbox_events WHERE status=? AND next_retry_at<=? ORDER BY created_at LIMIT ?`, outbox.StatusPending, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	items := []outbox.Item{}
	for rows.Next() {
		var i outbox.Item
		var next, created int64
		if err := rows.Scan(&i.ID, &i.EventType, &i.Destination, &i.Payload, &i.IdempotencyKey, &i.Status, &i.RetryCount, &next, &i.LastError, &created); err != nil {
			rows.Close()
			return nil, err
		}
		i.NextRetryAt = time.Unix(next, 0).UTC()
		i.CreatedAt = time.Unix(created, 0).UTC()
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, i := range items {
		res, err := tx.ExecContext(ctx, `UPDATE outbox_events SET status=?,updated_at=? WHERE id=? AND status=?`, outbox.StatusProcessing, now.Unix(), i.ID, outbox.StatusPending)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return nil, fmt.Errorf("outbox claim lost for %s", i.ID)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (d *DB) MarkOutboxDelivered(ctx context.Context, id string) error {
	_, err := d.ExecContext(ctx, `UPDATE outbox_events SET status=?,updated_at=? WHERE id=?`, outbox.StatusDelivered, time.Now().Unix(), id)
	return err
}
func (d *DB) RetryOutbox(ctx context.Context, id, reason string, next time.Time, dead bool) error {
	status := outbox.StatusPending
	if dead {
		status = outbox.StatusDeadLetter
	}
	_, err := d.ExecContext(ctx, `UPDATE outbox_events SET status=?,retry_count=retry_count+1,next_retry_at=?,last_error=?,updated_at=? WHERE id=?`, status, next.Unix(), reason, time.Now().Unix(), id)
	return err
}
