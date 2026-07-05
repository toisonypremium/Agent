package research

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"btc-agent/internal/config"
)

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	Link    []atomLink `xml:"link"`
	Summary string     `xml:"summary"`
	Updated string     `xml:"updated"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

func FetchRSS(ctx context.Context, cfg config.Config) ([]ResearchItem, int, []string) {
	items := []ResearchItem{}
	warnings := []string{}
	sourcesOK := 0
	if !cfg.Research.RSS.Enabled {
		return items, sourcesOK, warnings
	}
	feeds := cfg.Research.RSS.Feeds
	limit := cfg.Research.MaxSourcesPerCycle
	if limit <= 0 {
		limit = 20
	}
	timeout := time.Duration(cfg.Research.RequestTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	seen := map[string]bool{}
	for _, feedURL := range feeds {
		if len(items) >= limit {
			break
		}
		feedItems, err := fetchFeed(ctx, client, feedURL)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("rss feed failed %s: %v", feedURL, err))
			continue
		}
		sourcesOK++
		for _, item := range feedItems {
			if len(items) >= limit {
				break
			}
			key := strings.TrimSpace(item.URL)
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(item.Title))
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			item.Tags, item.Risk = classify(item.Title + " " + item.Summary)
			items = append(items, item)
		}
	}
	return items, sourcesOK, warnings
}

func fetchFeed(ctx context.Context, client *http.Client, feedURL string) ([]ResearchItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "btc-agent-research/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	items, err := parseRSS(data, feedURL)
	if err == nil && len(items) > 0 {
		return items, nil
	}
	atomItems, atomErr := parseAtom(data, feedURL)
	if atomErr == nil {
		return atomItems, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, atomErr
}

func parseRSS(data []byte, source string) ([]ResearchItem, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, err
	}
	if len(feed.Channel.Items) == 0 {
		return nil, fmt.Errorf("no rss items")
	}
	name := clean(feed.Channel.Title)
	if name == "" {
		name = source
	}
	out := []ResearchItem{}
	for _, item := range feed.Channel.Items {
		out = append(out, ResearchItem{Source: name, Title: clean(item.Title), URL: strings.TrimSpace(item.Link), PublishedAt: parseTime(item.PubDate), Summary: clean(item.Description), Risk: RiskInfo})
	}
	return out, nil
}

func parseAtom(data []byte, source string) ([]ResearchItem, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, err
	}
	if len(feed.Entries) == 0 {
		return nil, fmt.Errorf("no atom entries")
	}
	name := clean(feed.Title)
	if name == "" {
		name = source
	}
	out := []ResearchItem{}
	for _, entry := range feed.Entries {
		url := ""
		if len(entry.Link) > 0 {
			url = strings.TrimSpace(entry.Link[0].Href)
		}
		out = append(out, ResearchItem{Source: name, Title: clean(entry.Title), URL: url, PublishedAt: parseTime(entry.Updated), Summary: clean(entry.Summary), Risk: RiskInfo})
	}
	return out, nil
}

func classify(text string) ([]string, string) {
	upper := strings.ToUpper(text)
	tags := []string{}
	for _, tag := range []string{"BTC", "ETH", "SOL", "RENDER", "OKX", "BINANCE", "SEC", "ETF", "FED"} {
		if strings.Contains(upper, tag) {
			tags = append(tags, tag)
		}
	}
	risk := RiskInfo
	lower := strings.ToLower(text)
	for _, word := range []string{"hack", "exploit", "withdrawal", "outage", "lawsuit", "inflation", "liquidation"} {
		if strings.Contains(lower, word) {
			risk = RiskWarn
			break
		}
	}
	return uniqueStrings(tags), risk
}

func clean(s string) string {
	s = html.UnescapeString(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 02 Jan 2006 15:04:05 -0700", "02 Jan 2006 15:04:05 MST"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
