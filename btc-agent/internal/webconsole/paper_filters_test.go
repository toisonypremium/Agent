package webconsole

import (
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/storage"
)

func TestPaperOrdersFilterUsesExplicitStatusAndTimeOnly(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "paper.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	if err := db.SaveOrders([]agent2.PaperOrder{{ID: "open", Timestamp: now.Add(-time.Hour), Symbol: "BTCUSDT", Status: "OPEN"}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.PaperOrdersFiltered(50, "OPEN", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Orders) != 1 || out.Orders[0].ID != "open" {
		t.Fatalf("out=%+v", out)
	}
	if _, err := svc.PaperOrdersFiltered(50, "open;drop", time.Time{}); err == nil {
		t.Fatal("invalid status accepted")
	}
}
