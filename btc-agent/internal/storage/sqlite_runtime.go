package storage

import (
	"btc-agent/internal/runtime/ownership"
	"context"
	"database/sql"
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
	_, err := d.ExecContext(ctx, `UPDATE execution_leases SET expires_at=0 WHERE name=? AND instance_id=? AND fencing_token=?`, lease.Name, lease.InstanceID, lease.FencingToken)
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
