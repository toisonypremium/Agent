package storage

import (
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/microstructure"
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

func TestMMCalibrationPersistsInRuntimeState(t *testing.T) {
	db, err := Open(t.TempDir() + "/calibration.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	state := microstructure.NewCalibrationState()
	state.Symbols["BTCUSDT"] = &microstructure.SymbolCalibration{TakerAnomalyZ: 1.65, Resolved: 8}
	if err := db.SaveMMCalibration(state, time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	got, err := db.MMCalibration()
	if err != nil || got.Symbols["BTCUSDT"] == nil || got.Symbols["BTCUSDT"].TakerAnomalyZ != 1.65 {
		t.Fatalf("calibration round trip failed: %+v err=%v", got, err)
	}
}

func TestDailyOpeningEquityImmutableWithinUTCDateAndResetsNextDate(t *testing.T) {
	path := t.TempDir() + "/daily-basis.db"
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	day1 := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	first, err := db.DailyOpeningEquity(day1, 100)
	if err != nil {
		t.Fatal(err)
	}
	sameDay, err := db.DailyOpeningEquity(day1.Add(20*time.Hour), 80)
	if err != nil {
		t.Fatal(err)
	}
	if first.Equity != 100 || sameDay.Equity != 100 {
		t.Fatalf("same-day basis changed: first=%+v later=%+v", first, sameDay)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	afterRestart, err := db.DailyOpeningEquity(day1.Add(21*time.Hour), 70)
	if err != nil || afterRestart.Equity != 100 {
		t.Fatalf("basis did not survive restart: %+v err=%v", afterRestart, err)
	}
	nextDay, err := db.DailyOpeningEquity(day1.Add(24*time.Hour), 70)
	if err != nil || nextDay.Equity != 70 {
		t.Fatalf("new UTC day did not reset basis: %+v err=%v", nextDay, err)
	}
}

func TestMMCalibrationSurvivesDatabaseRestart(t *testing.T) {
	path := t.TempDir() + "/calibration-restart.db"
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	state := microstructure.NewCalibrationState()
	state.Symbols["ETHUSDT"] = &microstructure.SymbolCalibration{TakerAnomalyZ: 1.75, Resolved: 16, Successes: 9}
	if err := db.SaveMMCalibration(state, time.Unix(20, 0)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.MMCalibration()
	if err != nil || got.Symbols["ETHUSDT"] == nil || got.Symbols["ETHUSDT"].TakerAnomalyZ != 1.75 || got.Symbols["ETHUSDT"].Resolved != 16 {
		t.Fatalf("calibration did not survive restart: %+v err=%v", got, err)
	}
}
