package storage

import (
	"btc-agent/internal/market"
	"path/filepath"
	"testing"
	"time"
)

func TestPendingExecutionMarkoutWindowsAndLatestClose(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "telemetry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO live_orders(client_order_id,inst_id,symbol,side,type,price,quantity,notional,status,source) VALUES('c1','BTC-USDT','BTCUSDT','BUY','limit',100,1,100,'FILLED','HERMES_OPERATOR')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO live_position_events(timestamp,client_order_id,inst_id,symbol,side,delta_quantity,fill_price,notional_delta,status) VALUES(1000,'c1','BTC-USDT','BTCUSDT','BUY',1,100,100,'FILLED')`)
	if err != nil {
		t.Fatal(err)
	}
	windows, err := db.PendingExecutionMarkoutWindows(time.Unix(5000, 0))
	if err != nil || len(windows) != 1 || windows[0].Symbol != "BTCUSDT" {
		t.Fatalf("windows=%+v err=%v", windows, err)
	}
	candle := market.Candle{Symbol: "BTCUSDT", Interval: "1m", OpenTime: time.Unix(1060, 0), CloseTime: time.Unix(1119, 0), Open: 101, High: 101, Low: 101, Close: 101, Volume: 1}
	if err = db.SaveCandles([]market.Candle{candle}); err != nil {
		t.Fatal(err)
	}
	latest, ok, err := db.LatestCandleCloseTime("btcusdt", "1m")
	if err != nil || !ok || latest.Unix() != 1119 {
		t.Fatalf("latest=%v ok=%v err=%v", latest, ok, err)
	}
}
