package hermesmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/market"
)

const researchRunSchema = `CREATE TABLE IF NOT EXISTS hermes_research_datasets(
 dataset_id TEXT PRIMARY KEY,
 symbol TEXT NOT NULL,
 interval TEXT NOT NULL,
 start_at INTEGER NOT NULL,
 end_at INTEGER NOT NULL,
 row_count INTEGER NOT NULL,
 source TEXT NOT NULL,
 completeness REAL NOT NULL,
 content_hash TEXT NOT NULL UNIQUE,
 created_at INTEGER NOT NULL,
 authority TEXT NOT NULL,
 payload_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hermes_research_datasets_symbol ON hermes_research_datasets(symbol,interval,end_at DESC);
CREATE TABLE IF NOT EXISTS hermes_research_runs(
 run_id TEXT PRIMARY KEY,
 hypothesis_id TEXT NOT NULL,
 hypothesis_version INTEGER NOT NULL,
 dataset_id TEXT NOT NULL,
 dataset_hash TEXT NOT NULL,
 code_hash TEXT NOT NULL,
 config_hash TEXT NOT NULL,
 engine_version TEXT NOT NULL,
 started_at INTEGER NOT NULL,
 ended_at INTEGER,
 status TEXT NOT NULL,
 artifact_path TEXT NOT NULL,
 error_class TEXT NOT NULL,
 authority TEXT NOT NULL,
 payload_json TEXT NOT NULL,
 FOREIGN KEY(dataset_id) REFERENCES hermes_research_datasets(dataset_id)
);
CREATE INDEX IF NOT EXISTS idx_hermes_research_runs_hypothesis ON hermes_research_runs(hypothesis_id,hypothesis_version,started_at DESC);
CREATE INDEX IF NOT EXISTS idx_hermes_research_runs_status ON hermes_research_runs(status,started_at DESC);`

var researchRunStatuses = map[string]bool{"QUEUED": true, "RUNNING": true, "PASS": true, "FAIL": true, "REJECTED": true}

type DatasetRecord struct {
	DatasetID    string    `json:"dataset_id"`
	Symbol       string    `json:"symbol"`
	Interval     string    `json:"interval"`
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
	RowCount     int       `json:"row_count"`
	Source       string    `json:"source"`
	Completeness float64   `json:"completeness"`
	ContentHash  string    `json:"content_hash"`
	CreatedAt    time.Time `json:"created_at"`
	Authority    string    `json:"authority"`
}
type ResearchRun struct {
	RunID             string     `json:"run_id"`
	HypothesisID      string     `json:"hypothesis_id"`
	HypothesisVersion int        `json:"hypothesis_version"`
	DatasetID         string     `json:"dataset_id"`
	DatasetHash       string     `json:"dataset_hash"`
	CodeHash          string     `json:"code_hash"`
	ConfigHash        string     `json:"config_hash"`
	EngineVersion     string     `json:"engine_version"`
	StartedAt         time.Time  `json:"started_at"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	Status            string     `json:"status"`
	ArtifactPath      string     `json:"artifact_path"`
	ErrorClass        string     `json:"error_class,omitempty"`
	Authority         string     `json:"authority"`
}
type DatasetBar struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}
type ResearchAudit struct {
	GeneratedAt      time.Time      `json:"generated_at"`
	Datasets         int            `json:"datasets"`
	Runs             int            `json:"runs"`
	RunsByStatus     map[string]int `json:"runs_by_status"`
	OrphanRuns       int            `json:"orphan_runs"`
	InvalidAuthority int            `json:"invalid_authority"`
	HashMismatches   int            `json:"hash_mismatches"`
	Quality          string         `json:"quality"`
	Limits           []string       `json:"limits,omitempty"`
	Authority        string         `json:"authority"`
}

func EnsureResearchRuns(db DB) error { _, err := db.Exec(researchRunSchema); return err }
func BuildDatasetRecord(symbol, interval, source string, bars []DatasetBar, completeness float64) (DatasetRecord, error) {
	d := DatasetRecord{Symbol: strings.ToUpper(strings.TrimSpace(symbol)), Interval: strings.ToLower(strings.TrimSpace(interval)), Source: strings.TrimSpace(source), RowCount: len(bars), Completeness: completeness, CreatedAt: time.Now().UTC(), Authority: "research_only"}
	if d.Symbol == "" || d.Interval == "" || d.Source == "" {
		return d, errors.New("dataset identity incomplete")
	}
	if completeness < 0 || completeness > 1 {
		return d, errors.New("dataset completeness out of range")
	}
	if len(bars) == 0 {
		return d, errors.New("dataset has no bars")
	}
	if err := ValidateDatasetBars(bars, time.Time{}); err != nil {
		return d, err
	}
	d.StartAt = bars[0].Timestamp
	d.EndAt = bars[len(bars)-1].Timestamp
	d.ContentHash = HashDatasetBars(bars)
	d.DatasetID = "ds:" + d.ContentHash[:24]
	return d, nil
}
func ValidateDatasetBars(bars []DatasetBar, cutoff time.Time) error {
	var last time.Time
	for i, b := range bars {
		vals := []float64{b.Open, b.High, b.Low, b.Close, b.Volume}
		for _, v := range vals {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("bar %d contains non-finite value", i)
			}
		}
		if b.Timestamp.IsZero() {
			return fmt.Errorf("bar %d missing timestamp", i)
		}
		if !cutoff.IsZero() && b.Timestamp.After(cutoff) {
			return fmt.Errorf("bar %d exceeds evaluation cutoff", i)
		}
		if !last.IsZero() && !b.Timestamp.After(last) {
			return fmt.Errorf("bar %d timestamp is duplicate or non-monotonic", i)
		}
		if b.Open <= 0 || b.High <= 0 || b.Low <= 0 || b.Close <= 0 || b.Volume < 0 {
			return fmt.Errorf("bar %d has invalid price or volume", i)
		}
		if b.High < math.Max(math.Max(b.Open, b.Close), b.Low) || b.Low > math.Min(math.Min(b.Open, b.Close), b.High) {
			return fmt.Errorf("bar %d violates OHLC bounds", i)
		}
		last = b.Timestamp
	}
	return nil
}
func HashDatasetBars(bars []DatasetBar) string {
	h := sha256.New()
	for _, b := range bars {
		fmt.Fprintf(h, "%d|%.10g|%.10g|%.10g|%.10g|%.10g\n", b.Timestamp.UTC().UnixNano(), b.Open, b.High, b.Low, b.Close, b.Volume)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func HashCanonicalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		return "", err
	}
	b, err = json.Marshal(x)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func BuildDatasetFromCandles(symbol, interval, source string, candles []market.Candle, completeness float64) (DatasetRecord, error) {
	bars := make([]DatasetBar, len(candles))
	for i, c := range candles {
		bars[i] = DatasetBar{Timestamp: c.OpenTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume}
	}
	return BuildDatasetRecord(symbol, interval, source, bars, completeness)
}

func SaveDataset(db DB, d DatasetRecord) error {
	if err := EnsureResearchRuns(db); err != nil {
		return err
	}
	if d.DatasetID == "" || d.ContentHash == "" || d.RowCount <= 0 {
		return errors.New("dataset record incomplete")
	}
	if d.Authority != "research_only" {
		return errors.New("dataset authority must be research_only")
	}
	p, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO hermes_research_datasets(dataset_id,symbol,interval,start_at,end_at,row_count,source,completeness,content_hash,created_at,authority,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, d.DatasetID, d.Symbol, d.Interval, d.StartAt.Unix(), d.EndAt.Unix(), d.RowCount, d.Source, d.Completeness, d.ContentHash, d.CreatedAt.Unix(), d.Authority, string(p))
	return err
}
func NormalizeResearchRun(r ResearchRun) ResearchRun {
	r.Status = strings.ToUpper(strings.TrimSpace(r.Status))
	r.Authority = strings.ToLower(strings.TrimSpace(r.Authority))
	if r.Status == "" {
		r.Status = "QUEUED"
	}
	if r.Authority == "" {
		r.Authority = "research_only"
	}
	if r.HypothesisVersion <= 0 {
		r.HypothesisVersion = 1
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	if r.RunID == "" {
		parts := []string{r.HypothesisID, fmt.Sprint(r.HypothesisVersion), r.DatasetHash, r.CodeHash, r.ConfigHash}
		sort.Strings(parts)
		sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
		r.RunID = "run:" + hex.EncodeToString(sum[:])[:24]
	}
	return r
}
func SaveResearchRun(db DB, r ResearchRun) error {
	r = NormalizeResearchRun(r)
	if err := EnsureResearchRuns(db); err != nil {
		return err
	}
	if r.HypothesisID == "" || r.DatasetID == "" || r.DatasetHash == "" || r.CodeHash == "" || r.ConfigHash == "" || r.EngineVersion == "" {
		return errors.New("research run identity incomplete")
	}
	if !researchRunStatuses[r.Status] {
		return fmt.Errorf("unsupported research run status %q", r.Status)
	}
	if r.Authority != "research_only" {
		return errors.New("research run authority must be research_only")
	}
	if r.EndedAt != nil && r.EndedAt.Before(r.StartedAt) {
		return errors.New("research run ends before start")
	}
	p, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO hermes_research_runs(run_id,hypothesis_id,hypothesis_version,dataset_id,dataset_hash,code_hash,config_hash,engine_version,started_at,ended_at,status,artifact_path,error_class,authority,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.RunID, r.HypothesisID, r.HypothesisVersion, r.DatasetID, r.DatasetHash, r.CodeHash, r.ConfigHash, r.EngineVersion, r.StartedAt.Unix(), nullableTime(r.EndedAt), r.Status, r.ArtifactPath, r.ErrorClass, r.Authority, string(p))
	return err
}
func AuditResearch(db DB) (ResearchAudit, error) {
	a := ResearchAudit{GeneratedAt: time.Now().UTC(), RunsByStatus: map[string]int{}, Quality: "LIMITED", Authority: "research_only"}
	if err := EnsureResearchRuns(db); err != nil {
		return a, err
	}
	var err error
	a.Datasets, err = queryCount(db, `SELECT COUNT(*) FROM hermes_research_datasets`)
	if err != nil {
		return a, err
	}
	a.Runs, err = queryCount(db, `SELECT COUNT(*) FROM hermes_research_runs`)
	if err != nil {
		return a, err
	}
	a.OrphanRuns, err = queryCount(db, `SELECT COUNT(*) FROM hermes_research_runs r LEFT JOIN hermes_research_datasets d ON d.dataset_id=r.dataset_id WHERE d.dataset_id IS NULL`)
	if err != nil {
		return a, err
	}
	a.InvalidAuthority, err = queryCount(db, `SELECT (SELECT COUNT(*) FROM hermes_research_datasets WHERE authority<>'research_only')+(SELECT COUNT(*) FROM hermes_research_runs WHERE authority<>'research_only')`)
	if err != nil {
		return a, err
	}
	a.HashMismatches, err = queryCount(db, `SELECT COUNT(*) FROM hermes_research_runs r JOIN hermes_research_datasets d ON d.dataset_id=r.dataset_id WHERE r.dataset_hash<>d.content_hash`)
	if err != nil {
		return a, err
	}
	rows, err := db.Query(`SELECT status,COUNT(*) FROM hermes_research_runs GROUP BY status ORDER BY status`)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return a, err
		}
		a.RunsByStatus[status] = n
	}
	if err := rows.Close(); err != nil {
		return a, err
	}
	if a.Datasets == 0 {
		a.Limits = append(a.Limits, "no research datasets registered")
	}
	if a.Runs == 0 {
		a.Limits = append(a.Limits, "no research runs registered")
	}
	if a.OrphanRuns > 0 {
		a.Limits = append(a.Limits, "research runs reference missing datasets")
	}
	if a.InvalidAuthority > 0 {
		a.Limits = append(a.Limits, "research authority violation")
	}
	if a.HashMismatches > 0 {
		a.Limits = append(a.Limits, "research run dataset hash mismatch")
	}
	if a.OrphanRuns > 0 || a.InvalidAuthority > 0 || a.HashMismatches > 0 {
		a.Quality = "DEGRADED"
	} else if a.Datasets > 0 && a.Runs > 0 {
		a.Quality = "HEALTHY"
	}
	return a, nil
}

func EnsureResearchInvariants(db DB) error {
	if err := EnsureResearchRuns(db); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE TRIGGER IF NOT EXISTS hermes_research_run_authority_guard BEFORE INSERT ON hermes_research_runs WHEN NEW.authority <> 'research_only' BEGIN SELECT RAISE(ABORT,'research run authority must be research_only'); END;`)
	return err
}

// EnsureBaselineResearchPlan creates one explicit, deterministic research
// contract and queues runs against closed canonical datasets. It never executes
// a strategy and never exports evidence to the planner.
func EnsureBaselineResearchPlan(db DB, episodeID string) error {
	if strings.TrimSpace(episodeID) == "" {
		return errors.New("baseline plan requires source episode")
	}
	if err := EnsureHypotheses(db); err != nil {
		return err
	}
	if err := EnsureResearchRuns(db); err != nil {
		return err
	}
	var existing int
	rows, err := db.Query(`SELECT COUNT(*) FROM hermes_hypotheses WHERE status IN ('DRAFT','READY_FOR_RESEARCH','TESTING','MONITORING')`)
	if err != nil {
		return err
	}
	if rows.Next() {
		if err = rows.Scan(&existing); err != nil {
			rows.Close()
			return err
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if existing == 0 {
		h := NormalizeHypothesis(Hypothesis{
			Title:             "Closed daily reclaim baseline",
			Statement:         "A deterministic reclaim plus trend context should improve neutral forward return on closed daily candles.",
			FalsificationRule: "FALSIFY when strict out-of-sample forward return does not exceed the neutral baseline after costs.",
			Assumptions:       []string{"closed daily candles are complete", "canonical candle source is stable"},
			FeatureContract:   []string{"mm_quality", "trend"},
			Symbols:           []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"},
			Horizons:          []string{"1d", "3d", "7d"},
			RegimeScope:       []string{"ALL"}, Owner: "hermes-deterministic", SourceEpisodeID: episodeID,
			Status: "READY_FOR_RESEARCH", Authority: "research_only",
		})
		if _, err := SaveHypothesis(db, h, map[string]bool{"mm_quality": true, "trend": true}); err != nil {
			return err
		}
	}

	// Resolve the active hypothesis before opening the dataset cursor. Production
	// SQLite intentionally has one open connection, so nested queries would wait
	// forever for the connection held by the outer rows object.
	var hypothesisID string
	hRows, err := db.Query(`SELECT hypothesis_id FROM hermes_hypotheses WHERE status IN ('READY_FOR_RESEARCH','TESTING','MONITORING') ORDER BY updated_at DESC LIMIT 1`)
	if err != nil {
		return err
	}
	if hRows.Next() {
		if err = hRows.Scan(&hypothesisID); err != nil {
			hRows.Close()
			return err
		}
	}
	if err = hRows.Err(); err != nil {
		hRows.Close()
		return err
	}
	if err = hRows.Close(); err != nil {
		return err
	}
	if hypothesisID == "" {
		return errors.New("baseline hypothesis missing")
	}

	type baselineDataset struct {
		id, hash, symbol string
	}
	datasets := []baselineDataset{}
	rows, err = db.Query(`SELECT dataset_id,content_hash,symbol FROM hermes_research_datasets WHERE source='canonical_sqlite_candles_closed' ORDER BY symbol`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var dataset baselineDataset
		if err := rows.Scan(&dataset.id, &dataset.hash, &dataset.symbol); err != nil {
			rows.Close()
			return err
		}
		datasets = append(datasets, dataset)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}

	// Writes happen only after the read cursor releases the sole connection.
	for _, dataset := range datasets {
		r := NormalizeResearchRun(ResearchRun{HypothesisID: hypothesisID, HypothesisVersion: 1, DatasetID: dataset.id, DatasetHash: dataset.hash, CodeHash: "deterministic-baseline-v1", ConfigHash: "closed-daily-neutral-v1", EngineVersion: "hermes-research-v1", Status: "QUEUED", ArtifactPath: "reports/research/queued/" + dataset.symbol + "-baseline.json", Authority: "research_only"})
		if err := SaveResearchRun(db, r); err != nil {
			return err
		}
	}
	return nil
}
