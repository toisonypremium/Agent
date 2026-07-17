package main

import (
	"btc-agent/internal/storage"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildExecutionEvidenceReport(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "e.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,submitted_at,updated_at,source,strategy_version,config_hash) VALUES('c1','o1','BTC-USDT','BTCUSDT','BUY','limit',100,10,1000,'FILLED',100,110,'HERMES_OPERATOR','v1','abc')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO live_fills(client_order_id,order_id,inst_id,symbol,side,filled_quantity,avg_price,updated_at) VALUES('c1','o1','BTC-USDT','BTCUSDT','BUY',8,101,110)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO live_position_events(timestamp,client_order_id,order_id,inst_id,symbol,side,delta_quantity,fill_price,notional_delta,fee_delta,fee_currency,status) VALUES(100,'c1','o1','BTC-USDT','BTCUSDT','BUY',8,101,808,0,'USDT','FILLED')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []int{1, 5, 15, 60} {
		if _, err = db.Exec(`INSERT INTO execution_markouts(event_id,horizon_minutes,mark_price,markout_pct,measured_at) VALUES(1,?,?,?,1000)`, h, 102, .01); err != nil {
			t.Fatal(err)
		}
	}
	r, err := buildExecutionEvidenceReport(db, time.Unix(10000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalOrders != 1 || r.FillEvents != 1 || math.Abs(r.WeightedFillRatio-.8) > 1e-12 || r.CompletedMarkouts != 4 || r.MarkoutBacklog != 0 || len(r.Versions) != 1 || r.Versions[0].StrategyVersion != "v1" {
		t.Fatalf("bad report %+v", r)
	}
}
