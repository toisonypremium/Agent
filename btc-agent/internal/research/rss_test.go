package research

import (
	"strings"
	"testing"
)

func TestParseRSS(t *testing.T) {
	data := []byte(`<rss><channel><title>Test Feed</title><item><title>BTC hack at exchange</title><link>https://example.com/a</link><description>ETH and SOL liquidation risk</description><pubDate>Mon, 02 Jan 2006 15:04:05 +0000</pubDate></item></channel></rss>`)
	items, err := parseRSS(data, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "BTC hack at exchange" || items[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected items: %#v", items)
	}
	tags, risk := classify(items[0].Title + " " + items[0].Summary)
	if risk != RiskWarn {
		t.Fatalf("risk=%s want %s", risk, RiskWarn)
	}
	joined := strings.Join(tags, ",")
	for _, want := range []string{"BTC", "ETH", "SOL"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("tags=%v missing %s", tags, want)
		}
	}
}

func TestParseAtom(t *testing.T) {
	data := []byte(`<feed><title>Atom Feed</title><entry><title>Render ETF news</title><link href="https://example.com/b"/><summary>RENDER and BTC update</summary><updated>2026-07-05T12:00:00Z</updated></entry></feed>`)
	items, err := parseAtom(data, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URL != "https://example.com/b" {
		t.Fatalf("unexpected items: %#v", items)
	}
}
