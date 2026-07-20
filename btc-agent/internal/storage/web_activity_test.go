package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecentRuntimeEventsDoesNotMarkHandled(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "events.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	for i := 0; i < 3; i++ {
		if e = d.SaveRuntimeEvent(RuntimeEvent{Timestamp: time.Unix(int64(100+i), 0), Source: "test", Type: "TEST", Severity: "info", PayloadJSON: `{"summary":"ok"}`}); e != nil {
			t.Fatal(e)
		}
	}
	xs, e := d.RecentRuntimeEvents(2, 0)
	if e != nil || len(xs) != 2 {
		t.Fatalf("%v %d", e, len(xs))
	}
	var n int
	if e = d.QueryRow(`SELECT count(*) FROM runtime_events WHERE handled_at IS NOT NULL AND handled_at>0`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("writes=%d err=%v", n, e)
	}
}
