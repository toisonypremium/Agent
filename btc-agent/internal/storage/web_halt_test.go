package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRequestWebHaltIsOneWayAuditedAndIdempotent(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, e := d.RequestWebHalt("a@example.test", WebHaltReasonRuntimeIntegrity, "Unsafe state found", "0123456789abcdef", now)
	if e != nil || r.Duplicate {
		t.Fatalf("%+v %v", r, e)
	}
	r, e = d.RequestWebHalt("a@example.test", WebHaltReasonRuntimeIntegrity, "Unsafe state found", "0123456789abcdef", now.Add(time.Minute))
	if e != nil || !r.Duplicate {
		t.Fatalf("%+v %v", r, e)
	}
	h, e := d.IsHalted()
	if e != nil || !h {
		t.Fatal("halt not active")
	}
	var n int
	if e = d.QueryRow(`select count(*) from operator_audit_events where action='HALT'`).Scan(&n); e != nil || n != 1 {
		t.Fatalf("audit=%d err=%v", n, e)
	}
}
