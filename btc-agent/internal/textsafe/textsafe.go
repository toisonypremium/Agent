package textsafe

import (
	"strings"
)

// ContainsSecretLike reports whether text contains common secret markers.
func ContainsSecretLike(s string) bool {
	lower := strings.ToLower(s)
	for _, pat := range []string{"api_key", "api_secret", "passphrase value", "telegram_token", "bearer "} {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

// StripURLs removes URL-like tokens from text while preserving surrounding content.
func StripURLs(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		words := strings.Fields(line)
		kept := []string{}
		for _, word := range words {
			lower := strings.ToLower(strings.Trim(word, "()[]{}.,;"))
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
				continue
			}
			kept = append(kept, word)
		}
		lines[i] = strings.Join(kept, " ")
	}
	return strings.Join(lines, "\n")
}

// TrimTelegram trims text to a safe Telegram payload size.
func TrimTelegram(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s + "\n"
	}
	return strings.TrimSpace(s[:max]) + "\n...\n"
}

// TrimAtBoundary trims text at a newline or sentence boundary when possible.
func TrimAtBoundary(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := strings.LastIndex(s[:max], "\n")
	if cut < max/2 {
		cut = strings.LastIndex(s[:max], ".")
	}
	if cut < max/2 {
		cut = max
	}
	return strings.TrimSpace(s[:cut]) + "\n..."
}
