package storage

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHermesExecutionReceiptBlocksReplayAndMutationAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hermes-receipt.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HermesDecisionPayloadHash(map[string]any{"decision_id": "d1", "actions": []string{"probe"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReserveHermesExecution("d1", hash, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteHermesExecution("d1", HermesReceiptCompleted, "placed=1", time.Unix(101, 0)); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.ReserveHermesExecution("d1", hash, time.Unix(102, 0)); err == nil || !strings.Contains(err.Error(), "replay blocked") {
		t.Fatalf("replay accepted: %v", err)
	}
	other, _ := HermesDecisionPayloadHash(map[string]any{"decision_id": "d1", "actions": []string{"scale"}})
	if err := db.ReserveHermesExecution("d1", other, time.Unix(103, 0)); err == nil || !strings.Contains(err.Error(), "payload mismatch") {
		t.Fatalf("payload mutation accepted: %v", err)
	}
	receipt, err := db.HermesExecutionReceipt("d1")
	if err != nil || receipt.Status != HermesReceiptCompleted || receipt.PayloadHash != hash {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}

func TestHermesExecutionReceiptConcurrentSingleWinner(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "hermes-race.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hash, _ := HermesDecisionPayloadHash(map[string]string{"decision_id": "race"})
	var winners int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if db.ReserveHermesExecution("race", hash, time.Now()) == nil {
				atomic.AddInt32(&winners, 1)
			}
		}()
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("winners=%d want=1", winners)
	}
}
