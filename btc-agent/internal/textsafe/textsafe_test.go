package textsafe

import (
	"strings"
	"testing"
)

func TestStripURLs(t *testing.T) {
	in := "Tin A https://example.com/path vẫn giữ text\nwww.example.com bỏ link, giữ còn lại\n(http://x.test), ok"
	out := StripURLs(in)
	if strings.Contains(out, "https://") || strings.Contains(out, "http://") || strings.Contains(out, "www.") {
		t.Fatalf("expected URLs stripped, got %q", out)
	}
	for _, want := range []string{"Tin A", "vẫn giữ text", "bỏ link", "giữ còn lại", "ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in %q", want, out)
		}
	}
}

func TestContainsSecretLike(t *testing.T) {
	for _, in := range []string{"api_key=abc", "API_SECRET=abc", "telegram_token xyz", "Bearer token", "passphrase value abc"} {
		if !ContainsSecretLike(in) {
			t.Fatalf("expected secret-like marker for %q", in)
		}
	}
	if ContainsSecretLike("spot limit BUY post-only only") {
		t.Fatal("unexpected secret-like marker")
	}
}

func TestTrimTelegram(t *testing.T) {
	if got := TrimTelegram("abc", 10); got != "abc\n" {
		t.Fatalf("unexpected short trim: %q", got)
	}
	got := TrimTelegram("abcdef", 3)
	if got != "abc\n...\n" {
		t.Fatalf("unexpected trim: %q", got)
	}
}

func TestTrimAtBoundary(t *testing.T) {
	got := TrimAtBoundary("abc\ndef\nghi", 8)
	if got != "abc\ndef\n..." {
		t.Fatalf("unexpected boundary trim: %q", got)
	}
}
