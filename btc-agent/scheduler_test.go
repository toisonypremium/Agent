package main

import (
	"strings"
	"testing"
	"time"
)

func TestValidateSchedulerTelegramAI(t *testing.T) {
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
		t.Fatalf("expected valid output: %v", err)
	}
	if err := validateSchedulerTelegramAI(long + "..."); err == nil {
		t.Fatal("expected truncated output rejected")
	}
	if err := validateSchedulerTelegramAI(strings.ReplaceAll(long, "VI.", "")); err == nil {
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
