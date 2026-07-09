package usertext

import (
	"strings"
	"testing"
)

func TestTelegramVietnameseNormalizesTradingTerms(t *testing.T) {
	in := "Bot Alive | Status running | Live supervisor | Reconcile live orders | spot limit BUY post-only | no futures leverage market order | ACTIVE_LIMIT WATCH"
	got := TelegramVietnamese(in)
	for _, want := range []string{
		"Bot đang hoạt động",
		"Trạng thái",
		"Giám sát giao dịch thật",
		"Đối soát lệnh thật",
		"mua giao ngay bằng lệnh giới hạn tạo thanh khoản",
		"hợp đồng tương lai",
		"đòn bẩy",
		"lệnh thị trường",
		"ĐỦ ĐIỀU KIỆN ĐẶT LỆNH",
		"THEO DÕI",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	for _, bad := range []string{"Bot Alive", "Live supervisor", "Reconcile live orders", "spot limit BUY post-only", "futures", "leverage", "market order"} {
		if strings.Contains(got, bad) {
			t.Fatalf("still contains English term %q in %q", bad, got)
		}
	}
}

func TestTelegramVietnameseFixesCompositeWatchlist(t *testing.T) {
	got := TelegramVietnamese("III. WATCHLIST MM/LIQ")
	if strings.Contains(got, "THEO DÕILIST") {
		t.Fatalf("bad composite: %q", got)
	}
	if !strings.Contains(got, "DANH SÁCH THEO DÕI") {
		t.Fatalf("missing Vietnamese watchlist: %q", got)
	}
}
