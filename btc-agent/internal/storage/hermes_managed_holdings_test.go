package storage

import (
	"btc-agent/internal/exchange/live"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedHoldingBecomesHermesOwnedWithoutFillHistory(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := HermesManagedHolding{Symbol: "NEARUSDT", InstID: "NEAR-USDT", Quantity: 38.5, AvgEntryPrice: 2.1, Source: "OKX_ACCOUNT_ADOPTION"}
	if err := db.SaveHermesManagedHolding(h); err != nil {
		t.Fatal(err)
	}
	got, err := db.HermesOwnedPositions()
	if err != nil || len(got) != 1 || got[0].Symbol != "NEARUSDT" || got[0].Quantity != 38.5 || got[0].AvgEntryPrice != 2.1 {
		t.Fatalf("bad adopted ownership %+v err=%v", got, err)
	}
	if err = db.DeleteHermesManagedHolding("NEARUSDT"); err != nil {
		t.Fatal(err)
	}
	got, _ = db.HermesOwnedPositions()
	if len(got) != 0 {
		t.Fatalf("deleted holding remains %+v", got)
	}
}

func TestManagedHoldingDoesNotShrinkImmutableOwnedFill(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO live_orders(client_order_id,source) VALUES('hfill','HERMES_OPERATOR')`)
	if err != nil {
		t.Fatal(err)
	}
	e := live.LivePositionEvent{Timestamp: time.Now().Unix(), ClientOrderID: "hfill", InstID: "RENDER-USDT", Symbol: "RENDERUSDT", Side: "BUY", DeltaQuantity: 4, FillPrice: 1.5, NotionalDelta: 6}
	if err = db.SaveLivePositionEvent(e); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveHermesManagedHolding(HermesManagedHolding{Symbol: "RENDERUSDT", InstID: "RENDER-USDT", Quantity: 3, AvgEntryPrice: 1.4, Source: "OKX_ACCOUNT_ADOPTION"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.HermesOwnedPositions()
	if err != nil || len(got) != 1 || got[0].Quantity != 4 {
		t.Fatalf("immutable ownership shrunk %+v err=%v", got, err)
	}
}
