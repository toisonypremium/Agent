package research

import (
	"context"
	"time"

	"btc-agent/internal/config"
)

func BuildBrief(ctx context.Context, cfg config.Config) BriefResult {
	result := BriefResult{GeneratedAt: time.Now()}
	if !cfg.Research.Enabled {
		result.Warnings = append(result.Warnings, "research disabled")
		result.RefreshSummary()
		return result
	}
	if cfg.Research.RSS.Enabled {
		items, warnings := FetchRSS(ctx, cfg)
		result.Items = append(result.Items, items...)
		result.Warnings = append(result.Warnings, warnings...)
		result.SourcesChecked += len(cfg.Research.RSS.Feeds)
	}
	if len(result.Items) == 0 {
		result.Warnings = append(result.Warnings, "no research items collected")
	}
	result.RefreshSummary()
	return result
}
