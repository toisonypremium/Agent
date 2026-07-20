package hermesmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const hypothesisSchema = `CREATE TABLE IF NOT EXISTS hermes_hypotheses(
 hypothesis_id TEXT NOT NULL,
 version INTEGER NOT NULL,
 fingerprint TEXT NOT NULL,
 title TEXT NOT NULL,
 statement TEXT NOT NULL,
 falsification_rule TEXT NOT NULL,
 assumptions_json TEXT NOT NULL,
 feature_contract_json TEXT NOT NULL,
 symbols_json TEXT NOT NULL,
 horizons_json TEXT NOT NULL,
 regime_scope_json TEXT NOT NULL,
 owner TEXT NOT NULL,
 source_episode_id TEXT NOT NULL,
 status TEXT NOT NULL,
 supersedes_id TEXT,
 authority TEXT NOT NULL,
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 payload_json TEXT NOT NULL,
 PRIMARY KEY(hypothesis_id,version),
 UNIQUE(fingerprint)
);
CREATE INDEX IF NOT EXISTS idx_hermes_hypotheses_status ON hermes_hypotheses(status,updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_hermes_hypotheses_source ON hermes_hypotheses(source_episode_id);`

var hypothesisStatuses = map[string]bool{"DRAFT": true, "READY_FOR_RESEARCH": true, "TESTING": true, "MONITORING": true, "DECAYED": true, "REJECTED": true, "DISABLED": true}
var hypothesisHorizons = map[string]bool{"1h": true, "4h": true, "1d": true, "3d": true, "7d": true, "14d": true, "30d": true}

type Hypothesis struct {
	HypothesisID      string    `json:"hypothesis_id"`
	Version           int       `json:"version"`
	Fingerprint       string    `json:"fingerprint"`
	Title             string    `json:"title"`
	Statement         string    `json:"statement"`
	FalsificationRule string    `json:"falsification_rule"`
	Assumptions       []string  `json:"assumptions"`
	FeatureContract   []string  `json:"feature_contract"`
	Symbols           []string  `json:"symbols"`
	Horizons          []string  `json:"horizons"`
	RegimeScope       []string  `json:"regime_scope"`
	Owner             string    `json:"owner"`
	SourceEpisodeID   string    `json:"source_episode_id"`
	Status            string    `json:"status"`
	SupersedesID      string    `json:"supersedes_id,omitempty"`
	Authority         string    `json:"authority"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func EnsureHypotheses(db DB) error { _, err := db.Exec(hypothesisSchema); return err }
func NormalizeHypothesis(h Hypothesis) Hypothesis {
	h.Title = strings.TrimSpace(h.Title)
	h.Statement = collapse(h.Statement)
	h.FalsificationRule = collapse(h.FalsificationRule)
	h.Status = strings.ToUpper(strings.TrimSpace(h.Status))
	h.Owner = strings.TrimSpace(h.Owner)
	h.SourceEpisodeID = strings.TrimSpace(h.SourceEpisodeID)
	h.Authority = strings.ToLower(strings.TrimSpace(h.Authority))
	h.Symbols = normalizeUpper(h.Symbols)
	h.Horizons = normalizeLower(h.Horizons)
	h.FeatureContract = normalizeLower(h.FeatureContract)
	h.Assumptions = uniqueTrimmed(h.Assumptions)
	h.RegimeScope = normalizeUpper(h.RegimeScope)
	if h.Version <= 0 {
		h.Version = 1
	}
	if h.Status == "" {
		h.Status = "DRAFT"
	}
	if h.Authority == "" {
		h.Authority = "research_only"
	}
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now().UTC()
	}
	if h.UpdatedAt.IsZero() {
		h.UpdatedAt = h.CreatedAt
	}
	h.Fingerprint = hypothesisFingerprint(h)
	if h.HypothesisID == "" {
		h.HypothesisID = "hyp:" + h.Fingerprint[:24]
	}
	return h
}
func ValidateHypothesis(h Hypothesis, allowedFeatures map[string]bool) error {
	if h.Title == "" || h.Statement == "" || h.FalsificationRule == "" {
		return errors.New("hypothesis requires title, statement, and falsification rule")
	}
	if h.SourceEpisodeID == "" {
		return errors.New("hypothesis requires source episode")
	}
	if h.Authority != "research_only" {
		return errors.New("hypothesis authority must be research_only")
	}
	if !hypothesisStatuses[h.Status] {
		return fmt.Errorf("unsupported hypothesis status %q", h.Status)
	}
	if len(h.Symbols) == 0 || len(h.Horizons) == 0 {
		return errors.New("hypothesis requires symbols and horizons")
	}
	for _, v := range h.Horizons {
		if !hypothesisHorizons[v] {
			return fmt.Errorf("unsupported hypothesis horizon %q", v)
		}
	}
	if len(h.FeatureContract) == 0 {
		return errors.New("hypothesis requires feature contract")
	}
	for _, v := range h.FeatureContract {
		if allowedFeatures != nil && !allowedFeatures[v] {
			return fmt.Errorf("unregistered feature %q", v)
		}
	}
	if h.SupersedesID == h.HypothesisID {
		return errors.New("hypothesis cannot supersede itself")
	}
	return nil
}
func SaveHypothesis(db DB, h Hypothesis, allowedFeatures map[string]bool) (Hypothesis, error) {
	h = NormalizeHypothesis(h)
	if err := ValidateHypothesis(h, allowedFeatures); err != nil {
		return h, err
	}
	if err := EnsureHypotheses(db); err != nil {
		return h, err
	}
	payload, err := json.Marshal(h)
	if err != nil {
		return h, err
	}
	a, _ := json.Marshal(h.Assumptions)
	f, _ := json.Marshal(h.FeatureContract)
	s, _ := json.Marshal(h.Symbols)
	hz, _ := json.Marshal(h.Horizons)
	r, _ := json.Marshal(h.RegimeScope)
	_, err = db.Exec(`INSERT INTO hermes_hypotheses(hypothesis_id,version,fingerprint,title,statement,falsification_rule,assumptions_json,feature_contract_json,symbols_json,horizons_json,regime_scope_json,owner,source_episode_id,status,supersedes_id,authority,created_at,updated_at,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, h.HypothesisID, h.Version, h.Fingerprint, h.Title, h.Statement, h.FalsificationRule, string(a), string(f), string(s), string(hz), string(r), h.Owner, h.SourceEpisodeID, h.Status, h.SupersedesID, h.Authority, h.CreatedAt.Unix(), h.UpdatedAt.Unix(), string(payload))
	return h, err
}
func hypothesisFingerprint(h Hypothesis) string {
	parts := []string{strings.ToLower(collapse(h.Statement)), strings.ToLower(collapse(h.FalsificationRule)), strings.Join(h.FeatureContract, ","), strings.Join(h.Symbols, ","), strings.Join(h.Horizons, ",")}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
func normalizeUpper(v []string) []string {
	for i := range v {
		v[i] = strings.ToUpper(strings.TrimSpace(v[i]))
	}
	return sortedUnique(v)
}
func normalizeLower(v []string) []string {
	for i := range v {
		v[i] = strings.ToLower(strings.TrimSpace(v[i]))
	}
	return sortedUnique(v)
}
func uniqueTrimmed(v []string) []string {
	for i := range v {
		v[i] = strings.TrimSpace(v[i])
	}
	return sortedUnique(v)
}
func sortedUnique(v []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, x := range v {
		if x != "" && !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

type HypothesisAudit struct {
	GeneratedAt           time.Time      `json:"generated_at"`
	Total                 int            `json:"total"`
	ByStatus              map[string]int `json:"by_status"`
	MissingSourceEpisodes int            `json:"missing_source_episodes"`
	InvalidAuthority      int            `json:"invalid_authority"`
	Quality               string         `json:"quality"`
	Limits                []string       `json:"limits,omitempty"`
	Authority             string         `json:"authority"`
}

func AuditHypotheses(db DB) (HypothesisAudit, error) {
	a := HypothesisAudit{GeneratedAt: time.Now().UTC(), ByStatus: map[string]int{}, Quality: "LIMITED", Authority: "research_only"}
	if err := EnsureHypotheses(db); err != nil {
		return a, err
	}
	rows, err := db.Query(`SELECT status,COUNT(*) FROM hermes_hypotheses GROUP BY status ORDER BY status`)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return a, err
		}
		a.ByStatus[status] = count
		a.Total += count
	}
	if err := rows.Close(); err != nil {
		return a, err
	}
	a.MissingSourceEpisodes, err = queryCount(db, `SELECT COUNT(*) FROM hermes_hypotheses h LEFT JOIN hermes_cognitive_episodes e ON e.episode_id=h.source_episode_id WHERE e.episode_id IS NULL`)
	if err != nil {
		return a, err
	}
	a.InvalidAuthority, err = queryCount(db, `SELECT COUNT(*) FROM hermes_hypotheses WHERE authority<>'research_only'`)
	if err != nil {
		return a, err
	}
	if a.Total == 0 {
		a.Limits = append(a.Limits, "no hypotheses registered")
	}
	if a.MissingSourceEpisodes > 0 {
		a.Limits = append(a.Limits, "hypotheses reference missing source episodes")
	}
	if a.InvalidAuthority > 0 {
		a.Limits = append(a.Limits, "hypothesis authority violation")
	}
	if a.MissingSourceEpisodes > 0 || a.InvalidAuthority > 0 {
		a.Quality = "DEGRADED"
	} else if a.Total > 0 {
		a.Quality = "HEALTHY"
	}
	return a, nil
}
