package hermesmemory

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// OB1-inspired provenance and typed reasoning graph. It is local SQLite,
// service-process only, and never grants execution authority.
const graphSchema = `
CREATE TABLE IF NOT EXISTS hermes_provenance(
 provenance_id TEXT PRIMARY KEY,
 episode_id TEXT NOT NULL,
 parent_id TEXT,
 derivation_method TEXT NOT NULL,
 derivation_layer TEXT NOT NULL,
 supersedes_id TEXT,
 created_at INTEGER NOT NULL,
 metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_hermes_provenance_episode ON hermes_provenance(episode_id);
CREATE INDEX IF NOT EXISTS idx_hermes_provenance_parent ON hermes_provenance(parent_id);
CREATE TABLE IF NOT EXISTS hermes_reasoning_edges(
 edge_id INTEGER PRIMARY KEY AUTOINCREMENT,
 from_episode_id TEXT NOT NULL,
 to_episode_id TEXT NOT NULL,
 relation TEXT NOT NULL CHECK(relation IN ('supports','contradicts','evolved_into','supersedes','depends_on','related_to')),
 confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1),
 decay_weight REAL NOT NULL DEFAULT 1 CHECK(decay_weight >= 0 AND decay_weight <= 1),
 valid_from INTEGER NOT NULL,
 valid_until INTEGER,
 rationale TEXT NOT NULL DEFAULT '',
 classifier_version TEXT NOT NULL DEFAULT 'deterministic-v1',
 UNIQUE(from_episode_id,to_episode_id,relation)
);
CREATE INDEX IF NOT EXISTS idx_hermes_edges_outgoing ON hermes_reasoning_edges(from_episode_id,relation);
CREATE INDEX IF NOT EXISTS idx_hermes_edges_incoming ON hermes_reasoning_edges(to_episode_id,relation);
CREATE INDEX IF NOT EXISTS idx_hermes_edges_valid ON hermes_reasoning_edges(valid_until);`

var allowedRelations = map[string]bool{"supports": true, "contradicts": true, "evolved_into": true, "supersedes": true, "depends_on": true, "related_to": true}

type Provenance struct {
	ProvenanceID     string    `json:"provenance_id"`
	EpisodeID        string    `json:"episode_id"`
	ParentID         string    `json:"parent_id,omitempty"`
	DerivationMethod string    `json:"derivation_method"`
	DerivationLayer  string    `json:"derivation_layer"`
	SupersedesID     string    `json:"supersedes_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	MetadataJSON     string    `json:"metadata_json,omitempty"`
}
type ReasoningEdge struct {
	EdgeID            int64      `json:"edge_id"`
	FromEpisodeID     string     `json:"from_episode_id"`
	ToEpisodeID       string     `json:"to_episode_id"`
	Relation          string     `json:"relation"`
	Confidence        float64    `json:"confidence"`
	DecayWeight       float64    `json:"decay_weight"`
	ValidFrom         time.Time  `json:"valid_from"`
	ValidUntil        *time.Time `json:"valid_until,omitempty"`
	Rationale         string     `json:"rationale,omitempty"`
	ClassifierVersion string     `json:"classifier_version"`
}

func EnsureGraph(db DB) error { _, err := db.Exec(graphSchema); return err }
func SaveProvenance(db DB, p Provenance) error {
	if err := EnsureGraph(db); err != nil {
		return err
	}
	if p.EpisodeID == "" || p.DerivationMethod == "" || p.DerivationLayer == "" {
		return errors.New("provenance identity incomplete")
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO hermes_provenance(provenance_id,episode_id,parent_id,derivation_method,derivation_layer,supersedes_id,created_at,metadata_json) VALUES(?,?,?,?,?,?,?,?)`, p.ProvenanceID, p.EpisodeID, p.ParentID, p.DerivationMethod, p.DerivationLayer, p.SupersedesID, p.CreatedAt.Unix(), p.MetadataJSON)
	return err
}
func SaveReasoningEdge(db DB, e ReasoningEdge) error {
	if err := EnsureGraph(db); err != nil {
		return err
	}
	e.Relation = strings.ToLower(strings.TrimSpace(e.Relation))
	if e.FromEpisodeID == "" || e.ToEpisodeID == "" || e.FromEpisodeID == e.ToEpisodeID || !allowedRelations[e.Relation] {
		return errors.New("invalid reasoning edge")
	}
	if e.Confidence < 0 || e.Confidence > 1 || e.DecayWeight < 0 || e.DecayWeight > 1 {
		return errors.New("reasoning edge score out of range")
	}
	if e.ValidFrom.IsZero() {
		e.ValidFrom = time.Now().UTC()
	}
	if e.ClassifierVersion == "" {
		e.ClassifierVersion = "deterministic-v1"
	}
	_, err := db.Exec(`INSERT INTO hermes_reasoning_edges(from_episode_id,to_episode_id,relation,confidence,decay_weight,valid_from,valid_until,rationale,classifier_version) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(from_episode_id,to_episode_id,relation) DO UPDATE SET confidence=excluded.confidence,decay_weight=excluded.decay_weight,rationale=excluded.rationale`, e.FromEpisodeID, e.ToEpisodeID, e.Relation, e.Confidence, e.DecayWeight, e.ValidFrom.Unix(), nullableTime(e.ValidUntil), e.Rationale, e.ClassifierVersion)
	return err
}
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}
func CurrentEdges(db DB, episodeID string, limit int) ([]ReasoningEdge, error) {
	if err := EnsureGraph(db); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(`SELECT edge_id,from_episode_id,to_episode_id,relation,confidence,decay_weight,valid_from,valid_until,rationale,classifier_version FROM hermes_reasoning_edges WHERE (from_episode_id=? OR to_episode_id=?) AND (valid_until IS NULL OR valid_until>?) ORDER BY confidence*decay_weight DESC LIMIT ?`, episodeID, episodeID, time.Now().Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReasoningEdge{}
	for rows.Next() {
		var e ReasoningEdge
		var vf int64
		var vu sql.NullInt64
		if err := rows.Scan(&e.EdgeID, &e.FromEpisodeID, &e.ToEpisodeID, &e.Relation, &e.Confidence, &e.DecayWeight, &vf, &vu, &e.Rationale, &e.ClassifierVersion); err != nil {
			return nil, err
		}
		e.ValidFrom = time.Unix(vf, 0)
		if vu.Valid {
			x := time.Unix(vu.Int64, 0)
			e.ValidUntil = &x
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type BrainAudit struct {
	GeneratedAt        time.Time      `json:"generated_at"`
	Episodes           int            `json:"episodes"`
	ProvenanceRecords  int            `json:"provenance_records"`
	ProvenanceCoverage float64        `json:"provenance_coverage"`
	ReasoningEdges     int            `json:"reasoning_edges"`
	CurrentEdges       int            `json:"current_edges"`
	Relations          map[string]int `json:"relations"`
	OrphanProvenance   int            `json:"orphan_provenance"`
	SelfLoops          int            `json:"self_loops"`
	Quality            string         `json:"quality"`
	Limits             []string       `json:"limits,omitempty"`
	Authority          string         `json:"authority"`
}

// BackfillEpisodeProvenance is idempotent. Historical episodes get a generic
// lineage marker; richer records already written by the live cycle win.
func BackfillEpisodeProvenance(db DB) error {
	if err := EnsureGraph(db); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO hermes_provenance(provenance_id,episode_id,derivation_method,derivation_layer,created_at,metadata_json)
SELECT 'prov:' || episode_id, episode_id, 'historical_episode_backfill', 'derived', created_at, '{"authority":"deterministic_engine_only","source":"hermes_cognitive_episodes"}'
FROM hermes_cognitive_episodes`)
	return err
}

// AuditBrain follows OB1's read-only smoke-test pattern. Optional data scarcity
// lowers quality but does not fail the service or alter market authority.
func AuditBrain(db DB, currentEpisodeID string) (BrainAudit, error) {
	a := BrainAudit{GeneratedAt: time.Now().UTC(), Relations: map[string]int{}, Quality: "HEALTHY", Authority: "deterministic_engine_only"}
	if err := EnsureGraph(db); err != nil {
		return a, err
	}
	checks := []struct {
		query string
		dst   *int
	}{
		{`SELECT COUNT(*) FROM hermes_cognitive_episodes`, &a.Episodes},
		{`SELECT COUNT(*) FROM hermes_provenance`, &a.ProvenanceRecords},
		{`SELECT COUNT(*) FROM hermes_reasoning_edges`, &a.ReasoningEdges},
		{`SELECT COUNT(*) FROM hermes_reasoning_edges WHERE from_episode_id=to_episode_id`, &a.SelfLoops},
		{`SELECT COUNT(*) FROM hermes_provenance p LEFT JOIN hermes_cognitive_episodes e ON e.episode_id=p.episode_id WHERE e.episode_id IS NULL`, &a.OrphanProvenance},
	}
	for _, c := range checks {
		count, err := queryCount(db, c.query)
		if err != nil {
			return a, err
		}
		*c.dst = count
	}
	if a.Episodes > 0 {
		a.ProvenanceCoverage = float64(a.ProvenanceRecords) / float64(a.Episodes)
	}
	rows, err := db.Query(`SELECT relation,COUNT(*) FROM hermes_reasoning_edges GROUP BY relation ORDER BY relation`)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var relation string
		var count int
		if err := rows.Scan(&relation, &count); err != nil {
			rows.Close()
			return a, err
		}
		a.Relations[relation] = count
	}
	if err := rows.Close(); err != nil {
		return a, err
	}
	if currentEpisodeID != "" {
		count, err := queryCount(db, `SELECT COUNT(*) FROM hermes_reasoning_edges WHERE from_episode_id=? OR to_episode_id=?`, currentEpisodeID, currentEpisodeID)
		if err != nil {
			return a, err
		}
		a.CurrentEdges = count
	}
	if a.ProvenanceCoverage < 1 {
		a.Limits = append(a.Limits, "provenance coverage below 100%")
	}
	if a.SelfLoops > 0 {
		a.Limits = append(a.Limits, "reasoning graph contains self loops")
	}
	if a.OrphanProvenance > 0 {
		a.Limits = append(a.Limits, "provenance references missing episodes")
	}
	if a.Episodes < 20 {
		a.Limits = append(a.Limits, "fewer than 20 cognitive episodes")
	}
	if a.ReasoningEdges == 0 {
		a.Limits = append(a.Limits, "no typed reasoning edges")
	}
	if a.SelfLoops > 0 || a.OrphanProvenance > 0 {
		a.Quality = "DEGRADED"
	} else if len(a.Limits) > 0 {
		a.Quality = "LIMITED"
	}
	return a, nil
}

func queryCount(db DB, query string, args ...any) (int, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, sql.ErrNoRows
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, rows.Err()
}
