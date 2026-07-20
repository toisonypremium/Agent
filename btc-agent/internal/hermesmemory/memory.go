package hermesmemory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const schema = `CREATE TABLE IF NOT EXISTS hermes_cognitive_episodes(episode_id TEXT PRIMARY KEY,created_at INTEGER NOT NULL,regime TEXT NOT NULL,phase TEXT NOT NULL,permission TEXT NOT NULL,plan_state TEXT NOT NULL,doctor_status TEXT NOT NULL,trend REAL NOT NULL,mm_verdict TEXT NOT NULL,mm_quality REAL NOT NULL,confidence REAL NOT NULL,payload_json TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_hermes_cognitive_episodes_created ON hermes_cognitive_episodes(created_at DESC);`

type DB interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
}
type Situation struct {
	GeneratedAt            time.Time `json:"generated_at"`
	Regime                 string    `json:"regime"`
	Phase                  string    `json:"phase"`
	Permission             string    `json:"permission"`
	PlanState              string    `json:"plan_state"`
	DoctorStatus           string    `json:"doctor_status"`
	Trend                  float64   `json:"trend"`
	MMVerdict              string    `json:"mm_verdict,omitempty"`
	MMQuality              float64   `json:"mm_quality,omitempty"`
	AuditAgeMinutes        int       `json:"audit_age_minutes,omitempty"`
	ForcedSimulationPassed bool      `json:"forced_simulation_passed"`
	Authority              string    `json:"authority"`
}
type Episode struct {
	EpisodeID        string    `json:"episode_id"`
	CreatedAt        time.Time `json:"created_at"`
	Situation        Situation `json:"situation"`
	Facts            []string  `json:"facts"`
	Inferences       []string  `json:"inferences,omitempty"`
	Unknowns         []string  `json:"unknowns,omitempty"`
	Conclusion       string    `json:"conclusion"`
	Confidence       float64   `json:"confidence"`
	ConfidenceLimits []string  `json:"confidence_limits,omitempty"`
}
type SimilarEpisode struct {
	Episode    Episode `json:"episode"`
	Similarity float64 `json:"similarity"`
}
type Context struct {
	Similar              []SimilarEpisode `json:"similar,omitempty"`
	SampleCount          int              `json:"sample_count"`
	CalibratedConfidence float64          `json:"calibrated_confidence"`
	ConfidenceLimits     []string         `json:"confidence_limits,omitempty"`
	Contradictions       []string         `json:"contradictions,omitempty"`
	Rule                 string           `json:"rule"`
}

func Ensure(db DB) error { _, e := db.Exec(schema); return e }
func BuildEpisode(s Situation, conclusion string, facts, inferences, unknowns []string) Episode {
	c, l := CalibratedConfidence(s, 0)
	e := Episode{CreatedAt: time.Now().UTC(), Situation: s, Facts: unique(facts), Inferences: unique(inferences), Unknowns: unique(unknowns), Conclusion: strings.TrimSpace(conclusion), Confidence: c, ConfidenceLimits: l}
	b, _ := json.Marshal(struct {
		S Situation
		C string
	}{s, e.Conclusion})
	h := sha256.Sum256(append(b, []byte(e.CreatedAt.Format("2006-01-02T15:04"))...))
	e.EpisodeID = hex.EncodeToString(h[:16])
	return e
}
func Save(db DB, e Episode) error {
	if x := Ensure(db); x != nil {
		return x
	}
	b, x := json.Marshal(e)
	if x != nil {
		return x
	}
	_, x = db.Exec(`INSERT OR IGNORE INTO hermes_cognitive_episodes(episode_id,created_at,regime,phase,permission,plan_state,doctor_status,trend,mm_verdict,mm_quality,confidence,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, e.EpisodeID, e.CreatedAt.Unix(), e.Situation.Regime, e.Situation.Phase, e.Situation.Permission, e.Situation.PlanState, e.Situation.DoctorStatus, e.Situation.Trend, e.Situation.MMVerdict, e.Situation.MMQuality, e.Confidence, string(b))
	return x
}
func Recall(db DB, current Situation, limit int) (Context, error) {
	ctx := Context{Rule: "Memory is evidence, not authority. Never copy an old conclusion without checking current contradictory facts."}
	if limit <= 0 {
		limit = 5
	}
	if e := Ensure(db); e != nil {
		return ctx, e
	}
	rows, e := db.Query(`SELECT payload_json FROM hermes_cognitive_episodes ORDER BY created_at DESC LIMIT 200`)
	if e != nil {
		return ctx, e
	}
	defer rows.Close()
	all := []SimilarEpisode{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return ctx, e
		}
		var ep Episode
		if json.Unmarshal([]byte(raw), &ep) == nil {
			all = append(all, SimilarEpisode{ep, Similarity(current, ep.Situation)})
		}
	}
	if e = rows.Err(); e != nil {
		return ctx, e
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Similarity > all[j].Similarity })
	if len(all) > limit {
		all = all[:limit]
	}
	ctx.Similar = all
	// Repeated hourly observations of the same state are one evidence cluster,
	// not independent samples. Count diverse contexts to prevent confidence inflation.
	diverse := map[string]bool{}
	for _, item := range all {
		s := item.Episode.Situation
		trendBucket := int(s.Trend / 10)
		key := strings.Join([]string{s.Regime, s.Phase, s.Permission, s.PlanState, s.MMVerdict, s.DoctorStatus, fmt.Sprintf("%d", trendBucket)}, "|")
		diverse[key] = true
	}
	ctx.SampleCount = len(diverse)
	ctx.CalibratedConfidence, ctx.ConfidenceLimits = CalibratedConfidence(current, ctx.SampleCount)
	for _, m := range all {
		if m.Similarity >= .72 && strings.EqualFold(m.Episode.Situation.Regime, current.Regime) && !strings.EqualFold(m.Episode.Situation.Permission, current.Permission) {
			ctx.Contradictions = append(ctx.Contradictions, "Similar regime has conflicting permission: current="+current.Permission+", memory="+m.Episode.Situation.Permission)
		}
	}
	ctx.Contradictions = unique(ctx.Contradictions)
	return ctx, nil
}
func Similarity(a, b Situation) float64 {
	s, w := 0.0, 0.0
	eq := func(x, y string, z float64) {
		w += z
		if x != "" && strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(y)) {
			s += z
		}
	}
	eq(a.Regime, b.Regime, .22)
	eq(a.Phase, b.Phase, .20)
	eq(a.Permission, b.Permission, .14)
	eq(a.PlanState, b.PlanState, .10)
	eq(a.MMVerdict, b.MMVerdict, .10)
	eq(a.DoctorStatus, b.DoctorStatus, .08)
	w += .16
	s += .16 * math.Max(0, 1-math.Abs(a.Trend-b.Trend)/100)
	return s / w
}
func CalibratedConfidence(s Situation, n int) (float64, []string) {
	c := .75
	l := []string{}
	if !strings.EqualFold(s.DoctorStatus, "DOCTOR_OK") && !strings.EqualFold(s.DoctorStatus, "OK") {
		c = math.Min(c, .35)
		l = append(l, "data/doctor not healthy")
	}
	if s.AuditAgeMinutes > 30 {
		c = math.Min(c, .40)
		l = append(l, "audit stale")
	}
	if !s.ForcedSimulationPassed {
		c = math.Min(c, .45)
		l = append(l, "forced simulation not passed")
	}
	if s.MMQuality > 0 && s.MMQuality < .65 {
		c = math.Min(c, .50)
		l = append(l, "microstructure quality below 0.65")
	}
	if n < 3 {
		c = math.Min(c, .55)
		l = append(l, "fewer than 3 similar memories")
	}
	if strings.TrimSpace(s.Authority) == "" {
		c = math.Min(c, .35)
		l = append(l, "market authority unknown")
	}
	return c, unique(l)
}
func unique(in []string) []string {
	o := []string{}
	m := map[string]bool{}
	for _, x := range in {
		x = strings.TrimSpace(x)
		if x != "" && !m[x] {
			m[x] = true
			o = append(o, x)
		}
	}
	return o
}
