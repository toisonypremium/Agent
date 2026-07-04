package aiagent

import "strings"

type SafetyResult struct {
	Pass    bool     `json:"pass"`
	Reasons []string `json:"reasons,omitempty"`
}

func CheckSafety(text string, override bool) SafetyResult {
	lower := stripSafeNegations(strings.ToLower(text))
	reasons := []string{}
	if override {
		reasons = append(reasons, "OVERRIDE_ENGINE")
	}
	unsafeTerms := map[string]string{
		"futures":                "FUTURES",
		"leverage":               "LEVERAGE",
		"market order":           "MARKET_ORDER",
		"all-in":                 "ALL_IN",
		"override deterministic": "OVERRIDE_DETERMINISTIC",
		"ignore deterministic":   "IGNORE_DETERMINISTIC",
		"mua ngay":               "BUY_NOW",
		"vào lệnh ngay":          "ENTER_NOW",
		"vao lenh ngay":          "ENTER_NOW",
		"đặt lệnh thật":          "REAL_ORDER",
		"dat lenh that":          "REAL_ORDER",
	}
	for term, reason := range unsafeTerms {
		if strings.Contains(lower, term) {
			reasons = append(reasons, reason)
		}
	}
	return SafetyResult{Pass: len(reasons) == 0, Reasons: unique(reasons)}
}

func stripSafeNegations(text string) string {
	phrases := []string{
		"no futures", "no leverage", "no market order", "no market orders", "no real trading", "no real trade",
		"no use of futures", "no use of leverage", "no use of market orders", "no use of market order",
		"without futures", "without leverage", "without market orders", "without market order",
		"do not use futures", "do not use leverage", "do not use market orders", "do not use market order",
		"do not override", "does not override", "must not override", "not override",
		"không futures", "khong futures", "không dùng futures", "khong dung futures",
		"không leverage", "khong leverage", "không dùng leverage", "khong dung leverage",
		"không market order", "khong market order", "không dùng market order", "khong dung market order",
		"không vào lệnh", "khong vao lenh", "không đặt lệnh", "khong dat lenh", "không đặt lệnh thật", "khong dat lenh that",
		"không mua", "khong mua", "chưa đủ điều kiện vào lệnh", "chua du dieu kien vao lenh", "không đủ để vào lệnh", "khong du de vao lenh",
	}
	for _, p := range phrases {
		text = strings.ReplaceAll(text, p, "")
	}
	text = strings.ReplaceAll(text, "no futures, leverage, market orders", "")
	text = strings.ReplaceAll(text, "no futures, leverage or market orders", "")
	text = strings.ReplaceAll(text, "no futures/leverage/market orders", "")
	return text
}

func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
