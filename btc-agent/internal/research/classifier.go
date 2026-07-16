package research

import "strings"

var macroKeywords = []string{
	"federal reserve", "fed", "interest rate", "rate hike", "rate cut", "inflation", "cpi", "pce", "nfp", "gdp", "treasury", "yield", "dollar index", "quantitative easing", "quantitative tightening", " liquidity",
}

var policyKeywords = []string{
	"regulation", "regulatory", "sec", "cftc", "policy", "legislation", "clarity act", "election", "law", "ban",
}

var tradeKeywords = []string{
	"tariff", "trade war", "trade deal", "export", "import", "sanction", "geopolit", "middle east", "war", "conflict",
}

func classifyCategory(text string) string {
	lower := strings.ToLower(text)
	if containsKeyword(lower, macroKeywords) {
		return "macro"
	}
	if containsKeyword(lower, policyKeywords) {
		return "policy"
	}
	if containsKeyword(lower, tradeKeywords) {
		return "trade"
	}
	return "crypto"
}

func containsKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func sourceConfidence(source, url string) float64 {
	value := strings.ToLower(source + " " + url)
	switch {
	case strings.Contains(value, "federalreserve.gov"), strings.Contains(value, "imf.org"), strings.Contains(value, "worldbank.org"):
		return 1
	case strings.Contains(value, "reuters"), strings.Contains(value, "wsj"), strings.Contains(value, "dow jones"), strings.Contains(value, "bloomberg"):
		return 0.9
	case strings.Contains(value, "coindesk"), strings.Contains(value, "cointelegraph"), strings.Contains(value, "bbc"), strings.Contains(value, "economist"):
		return 0.7
	default:
		return 0.3
	}
}
