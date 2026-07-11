package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/paper"
	"btc-agent/internal/storage"
)

func TestEnvFileConflictWarningsDetectsModeAndAllowMismatch(t *testing.T) {
	files := []liveguard.EnvFileStatus{
		{Path: filepath.Join(t.TempDir(), "btc-agent.env"), Exists: true, Mode: "live-auto", AutoLiveAllow: "true", OKXKeyPresent: true, OKXSecretPresent: true, OKXPassphrasePresent: true},
		{Path: filepath.Join(t.TempDir(), ".env"), Exists: true, Mode: "paper", AutoLiveAllow: "false", OKXKeyPresent: true, OKXSecretPresent: true, OKXPassphrasePresent: true},
	}
	warnings := envFileConflictWarnings(files)
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"BTC_AGENT_MODE differs", "BTC_AGENT_ALLOW_AUTO_LIVE differs"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in warnings: %v", want, warnings)
		}
	}
	for _, leak := range []string{"OKX_API_KEY=", "OKX_API_SECRET=", "OKX_API_PASSPHRASE="} {
		if strings.Contains(joined, leak) {
			t.Fatalf("warning leaked secret-like assignment %q: %s", leak, joined)
		}
	}
}

func TestPersistManagedCycleResultKeepsDesiredExpiry(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	expires := time.Unix(1700003600, 0)
	result := liveguard.ManagedCycleResult{Placed: []liveguard.ManagedOrderDecision{{
		Reason: "test place",
		Desired: liveguard.ManagedDesiredOrder{
			Symbol:            "ETHUSDT",
			InstID:            "ETH-USDT",
			LayerIndex:        2,
			Side:              "BUY",
			Type:              "limit",
			Price:             100,
			Quantity:          0.02,
			Notional:          2,
			InvalidationPrice: 90,
			Source:            "deterministic_agent2_layer_2",
			DecisionReason:    "active",
			ExpiresAt:         expires,
		},
		PlaceResult: live.OrderResult{InstID: "ETH-USDT", OrderID: "ord-1", ClientOrderID: "client-1", Submitted: true},
	}}}
	if err := persistManagedCycleResult(db, result); err != nil {
		t.Fatalf("persist managed cycle: %v", err)
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("open orders=%d", len(open))
	}
	if open[0].ExpiresAt != expires.Unix() {
		t.Fatalf("expires_at=%d want %d", open[0].ExpiresAt, expires.Unix())
	}
}

func TestRunPaperManagerUpdatesOrderAndWritesReports(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	db, err := storage.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Unix(1700000000, 0)
	plan := agent2.Plan{Timestamp: now, State: agent2.StateActiveLimit, ActionPermission: agent1.Allowed, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1}}}}}
	if err := db.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	order := agent2.PaperOrder{ID: "paper-1", Timestamp: now, Symbol: "ETHUSDT", Side: "BUY", Layer: 1, Price: 100, Quantity: 1, Notional: 100, Status: "OPEN", ExpiresAt: now.Add(48 * time.Hour), InvalidationPrice: 90, Reason: "test"}
	if err := db.SaveOrders([]agent2.PaperOrder{order}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCandles([]market.Candle{{Symbol: "ETHUSDT", Interval: "1d", OpenTime: now.Add(24 * time.Hour), CloseTime: now.Add(48 * time.Hour), Open: 105, High: 106, Low: 99, Close: 104, Volume: 1000}}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.Data.CandleLimit = 10
	if err := runPaperManager(cfg, db); err != nil {
		t.Fatalf("run paper manager: %v", err)
	}
	counts, err := db.PaperOrderStatusCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts[paper.StatusFilled] != 1 {
		t.Fatalf("expected filled count, got %+v", counts)
	}
	for _, name := range []string{"paper_manager_latest.md", "paper_manager_latest.json"} {
		if _, err := os.Stat(filepath.Join("reports", name)); err != nil {
			t.Fatalf("missing report %s: %v", name, err)
		}
	}
}

func TestValidateSchedulerTelegramAI(t *testing.T) {
	base := `📊 BTC Agent — Tóm tắt chiến lược
I. Kết luận: không đặt lệnh vì BTC WATCH và chưa có ACTIVE_LIMIT. Không chase giá. Blocker chính: BTC permission WATCH. BTC 62840 | trend 19.8 | DOWNTREND/WATCH | plan WATCH
II. BTC & Kịch bản: Bias W/D/4H giảm/giảm/RANGE | Flow NEUTRAL 0.00 | risk vừa | Vùng active 57800–59775 | chính=Bảo toàn vốn | mở khóa=Cần reclaim/flow rõ | vô hiệu=mất support | Cần: Trend score cần tăng 25.2 điểm.
III. Watchlist MM/Liq: ETHUSDT 49% | MM=NO_EDGE 20 (chưa reclaim) | Liq=A 100 | gap 12.0% RR 2.17 | Chờ BTC chuyển ALLOWED; asset chỉ nằm watchlist, không tạo lệnh.
IV. Bot & Safety: Không ACTIVE_LIMIT: không đặt lệnh, không chase. WATCH không tạo probe. Runtime: MANAGED_CYCLE_COMPLETED desired=0 đặt=0. Spot limit BUY post-only only; không futures, không leverage, không market order.
`
	long := base + strings.Repeat("Nội dung phân tích bổ sung bằng tiếng Việt để vượt ngưỡng độ dài kiểm tra. ", 20)
	if err := validateSchedulerTelegramAI(long); err != nil {
		t.Fatalf("expected valid output: %v", err)
	}
	if err := validateSchedulerTelegramAI(long + "..."); err == nil {
		t.Fatal("expected truncated output rejected")
	}
	if err := validateSchedulerTelegramAI(strings.ReplaceAll(long, "IV.", "")); err == nil {
		t.Fatal("expected missing section rejected")
	}
	if err := validateSchedulerTelegramAI(strings.ReplaceAll(long, "không market", "")); err == nil {
		t.Fatal("expected missing safety rejected")
	}
}

func TestGetNextRunTime(t *testing.T) {
	// Setup timezone locations
	hcm, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	t.Run("daily run time is later today", func(t *testing.T) {
		// Now: 2026-07-04 06:00:00 ICT
		now := time.Date(2026, 7, 4, 6, 0, 0, 0, hcm)
		dailyRunTime := "08:00"

		got, err := getNextRunTime(dailyRunTime, hcm, now)
		if err != nil {
			t.Fatal(err)
		}

		expected := time.Date(2026, 7, 4, 8, 0, 0, 0, hcm)
		if !got.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, got)
		}
	})

	t.Run("daily run time is earlier today", func(t *testing.T) {
		// Now: 2026-07-04 10:00:00 ICT
		now := time.Date(2026, 7, 4, 10, 0, 0, 0, hcm)
		dailyRunTime := "08:00"

		got, err := getNextRunTime(dailyRunTime, hcm, now)
		if err != nil {
			t.Fatal(err)
		}

		expected := time.Date(2026, 7, 5, 8, 0, 0, 0, hcm)
		if !got.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, got)
		}
	})

	t.Run("daily run time is exactly now", func(t *testing.T) {
		// Now: 2026-07-04 08:00:00 ICT
		now := time.Date(2026, 7, 4, 8, 0, 0, 0, hcm)
		dailyRunTime := "08:00"

		got, err := getNextRunTime(dailyRunTime, hcm, now)
		if err != nil {
			t.Fatal(err)
		}

		expected := time.Date(2026, 7, 5, 8, 0, 0, 0, hcm)
		if !got.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, got)
		}
	})

	t.Run("invalid daily run time format", func(t *testing.T) {
		now := time.Now()
		invalid := []string{"invalid", "25:00", "08:99", "8", "8:00", "08:0", "aa:00"}
		for _, value := range invalid {
			t.Run(value, func(t *testing.T) {
				_, err := getNextRunTime(value, time.UTC, now)
				if err == nil {
					t.Error("expected error for invalid format")
				}
			})
		}
	})
}

func TestValidateSchedulerTelegramAIRejectsVagueWatchReport(t *testing.T) {
	base := `📊 BTC Agent — Bản tin chiến lược
I. Kết luận: không đặt lệnh vì BTC WATCH và chưa có ACTIVE_LIMIT. Không chase giá.
II. Phân tích kỹ thuật BTC: giá, regime, trend score, bias tuần/ngày/4H, flow score và risk đều được trình bày đủ để chủ tài khoản hiểu vì sao bot đứng ngoài lúc này.
III. Vùng giá & kịch bản: Kịch bản chính giữ vốn. Kịch bản mở khóa cần reclaim. Kịch bản vô hiệu là mất support.
IV. Kế hoạch bot: permission WATCH, plan WATCH. ETHUSDT: MM=NO_EDGE 10/100, Liq=D 22/100, Discount=12%, RR=2.2, thiếu=chưa reclaim, trigger=Chờ sweep low + close reclaim.
V. Research context: tin tức chỉ là bối cảnh phụ, không override Agent 1/2, không dùng URL trong Telegram.
VI. Trạng thái an toàn: daily OK, reconcile OK, supervisor OK. An toàn: spot limit BUY post-only only; không futures, không leverage, không market order.
`
	long := base + strings.Repeat("Nội dung phân tích bổ sung bằng tiếng Việt để vượt ngưỡng độ dài kiểm tra. ", 20)
	if err := validateSchedulerTelegramAI(long); err != nil {
		t.Fatalf("expected valid detailed output: %v", err)
	}
	if err := validateSchedulerTelegramAI(strings.ReplaceAll(long, "MM=NO_EDGE", "MM footprint")); err != nil {
		t.Fatalf("expected MM footprint wording accepted: %v", err)
	}
	if err := validateSchedulerTelegramAI(strings.ReplaceAll(long, "MM=NO_EDGE", "footprint")); err == nil {
		t.Fatal("expected missing MM detail rejected")
	}
	if err := validateSchedulerTelegramAI(strings.ReplaceAll(long, "Liq=D", "thanh khoản")); err == nil {
		t.Fatal("expected missing liquidity detail rejected")
	}
	if err := validateSchedulerTelegramAI(strings.ReplaceAll(long, "trigger=Chờ sweep low + close reclaim.", "theo dõi thêm.")); err == nil {
		t.Fatal("expected missing actionable trigger rejected")
	}
	if err := validateSchedulerTelegramAI(long + " https://example.com"); err == nil {
		t.Fatal("expected URL rejected")
	}
}
