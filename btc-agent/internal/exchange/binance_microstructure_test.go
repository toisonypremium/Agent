package exchange

import (
	"encoding/json"
	"testing"
)

func TestRawStringFloat(t *testing.T) {
	raw, _ := json.Marshal("123.45")
	if got := rawStringFloat(raw); got != 123.45 {
		t.Fatalf("unexpected string float: %.2f", got)
	}
	raw, _ = json.Marshal(67.89)
	if got := rawStringFloat(raw); got != 67.89 {
		t.Fatalf("unexpected number float: %.2f", got)
	}
}

func TestParseFloatDefault(t *testing.T) {
	if got := parseFloatDefault("1.25"); got != 1.25 {
		t.Fatalf("unexpected float: %.2f", got)
	}
	if got := parseFloatDefault("bad"); got != 0 {
		t.Fatalf("bad float should be zero: %.2f", got)
	}
}
