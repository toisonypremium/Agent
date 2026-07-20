package r2

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
)

var safePart = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitize(v string) string {
	v = safePart.ReplaceAllString(strings.TrimSpace(v), "-")
	v = strings.Trim(v, "-.")
	if v == "" {
		return "unknown"
	}
	return v
}
func ReportKey(at time.Time, reportID, ext string) string {
	return path.Join("reports", fmt.Sprintf("%04d", at.UTC().Year()), fmt.Sprintf("%02d", at.UTC().Month()), fmt.Sprintf("%02d", at.UTC().Day()), sanitize(reportID)+"."+sanitize(ext))
}
func ChartKey(asset string, at time.Time, chartID string) string {
	return path.Join("charts", sanitize(strings.ToLower(asset)), fmt.Sprintf("%04d", at.UTC().Year()), fmt.Sprintf("%02d", at.UTC().Month()), sanitize(chartID)+".png")
}
func BacktestKey(strategy, runID string) string {
	return path.Join("backtests", sanitize(strategy), sanitize(runID), "result.json")
}
