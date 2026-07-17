package storage

import (
	"btc-agent/internal/exchange/live"
	"testing"
	"time"
)

func TestEquityHighWaterPersists(t *testing.T) {
	db, e := Open(t.TempDir() + "/state.db")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.UpdateEquityRiskState(100, time.Unix(1, 0)); e != nil {
		t.Fatal(e)
	}
	s, e := db.UpdateEquityRiskState(80, time.Unix(2, 0))
	if e != nil {
		t.Fatal(e)
	}
	if s.HighWaterMark != 100 || s.UnrealizedDrawdown != 20 || s.DrawdownPct != .2 {
		t.Fatalf("bad state %+v", s)
	}
}
func TestExitPeaksPersist(t *testing.T) {
	db, e := Open(t.TempDir() + "/peak.db")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = db.SaveExitPeakStates([]ExitPeakState{{Symbol: "ethusdt", Peak: 123, TrailActive: true, UpdatedAt: time.Unix(3, 0)}}); e != nil {
		t.Fatal(e)
	}
	x, e := db.ExitPeakStates()
	if e != nil || len(x) != 1 || x[0].Symbol != "ETHUSDT" || !x[0].TrailActive {
		t.Fatalf("bad peaks %+v %v", x, e)
	}
}
func TestReplayChecksumStable(t *testing.T) {
	db, e := Open(t.TempDir() + "/replay.db")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`INSERT INTO live_orders(client_order_id,inst_id,symbol,side,type,price,quantity,notional,status,source) VALUES('x','ETH-USDT','ETHUSDT','BUY','limit',100,1,100,?,'HERMES_OPERATOR')`, live.StatusFilled)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.SaveLivePositionEvent(live.LivePositionEvent{Timestamp: 1, ClientOrderID: "x", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: 1, FillPrice: 100, NotionalDelta: 100}); e != nil {
		t.Fatal(e)
	}
	a, e := db.ReplayHermesState()
	if e != nil {
		t.Fatal(e)
	}
	b, e := db.ReplayHermesState()
	if e != nil || a.Checksum != b.Checksum || a.Events != 1 {
		t.Fatalf("unstable replay %+v %+v %v", a, b, e)
	}
}
