package research

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"btc-agent/internal/config"
)

func RunDoctor(ctx context.Context, cfg config.Config) DoctorResult {
	result := DoctorResult{GeneratedAt: time.Now()}
	if !cfg.Research.Enabled {
		result.Warnings = append(result.Warnings, "research disabled")
		result.Channels = append(result.Channels, ChannelStatus{Name: "rss", Enabled: cfg.Research.RSS.Enabled, Usable: false})
		result.RefreshSummary()
		return result
	}
	usable := 0
	if cfg.Research.RSS.Enabled {
		status := ChannelStatus{Name: "rss", Enabled: true, Checked: len(cfg.Research.RSS.Feeds)}
		client := &http.Client{Timeout: timeout(cfg)}
		for _, feedURL := range cfg.Research.RSS.Feeds {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
			if err != nil {
				status.Error = err.Error()
				continue
			}
			req.Header.Set("User-Agent", "btc-agent-research/1.0")
			resp, err := client.Do(req)
			if err != nil {
				status.Error = err.Error()
				continue
			}
			resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				status.Usable = true
				usable++
				break
			}
			status.Error = fmt.Sprintf("http %d", resp.StatusCode)
		}
		if !status.Usable && status.Error != "" {
			result.Warnings = append(result.Warnings, "rss unavailable: "+status.Error)
		}
		result.Channels = append(result.Channels, status)
	}
	if usable == 0 {
		result.Blockers = append(result.Blockers, "no research channel usable")
	}
	result.RefreshSummary()
	return result
}

func timeout(cfg config.Config) time.Duration {
	seconds := cfg.Research.RequestTimeoutSeconds
	if seconds <= 0 {
		seconds = 12
	}
	return time.Duration(seconds) * time.Second
}
