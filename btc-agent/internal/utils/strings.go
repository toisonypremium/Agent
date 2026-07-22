// Package utils provides shared utility helpers used across btc-agent packages.
package utils

// FirstNonEmpty returns the first non-empty string from values, or "" if all are empty.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// UniqueStrings returns a slice with duplicate strings removed, preserving order.
func UniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
