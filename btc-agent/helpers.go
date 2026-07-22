package main

import (
	"btc-agent/internal/reportio"
	"fmt"
	"strconv"
	"strings"
)

// saveJSONFile marshals v to JSON and writes it to dir/name.
// It is a thin root-level convenience wrapper around reportio.WriteJSON.
func saveJSONFile(dir, name string, v any) error {
	return reportio.WriteJSON(dir, name, v)
}

// firstNonEmpty returns the first non-empty string from values, or "".
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// uniqueStringsMain returns values with duplicates and empty strings removed.
func uniqueStringsMain(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// firstNonZero returns the first non-zero int from values, or 0.
func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

// firstStrings returns the first limit items from items (or all items if len <= limit).
func firstStrings(items []string, limit int) []string {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

// emptyDefault returns value if non-empty, otherwise fallback.
func emptyDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// intArgValue parses a non-negative integer from --flag value in args.
func intArgValue(args []string, key string) (int, error) {
	value := argValue(args, key)
	if value == "" {
		return 0, nil
	}
	out, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if out < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return out, nil
}

// floatArgValue parses a non-negative float64 from --flag value in args.
func floatArgValue(args []string, key string) (float64, error) {
	value := argValue(args, key)
	if value == "" {
		return 0, nil
	}
	out, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if out < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return out, nil
}

// argValue returns the value of the form --flag=value or the next arg after --flag.
func argValue(args []string, flag string) string {
	for i, a := range args {
		if strings.HasPrefix(a, flag+"=") {
			return strings.TrimPrefix(a, flag+"=")
		}
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// hasFlag returns true if flag appears in args.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
