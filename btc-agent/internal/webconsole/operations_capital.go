package webconsole

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const runtimeHealthFreshFor = 5 * time.Minute
const runtimeHealthArtifactName = "web_console_runtime_health.json"

type RuntimeHealthSnapshot struct {
	ObservedAt          time.Time `json:"observed_at"`
	SchedulerCount      int       `json:"scheduler_count"`
	HeartbeatState      string    `json:"heartbeat_state"`
	HeartbeatAgeSeconds int64     `json:"heartbeat_age_seconds"`
	LeaseInstanceID     string    `json:"lease_instance_id"`
	LeaseFresh          bool      `json:"lease_fresh"`
	DatabaseState       string    `json:"database_state"`
	ObserverState       string    `json:"observer_state"`
}

type RuntimeHealthSource interface {
	LoadRuntimeHealth() (RuntimeHealthSnapshot, error)
}
type runtimeHealthFile struct{ path string }

type runtimeHealthArtifact struct{ dir string }

// NewRuntimeHealthArtifact reads one fixed observer-produced JSON filename only.
func NewRuntimeHealthArtifact(dir string) RuntimeHealthSource { return runtimeHealthArtifact{dir: dir} }
func (a runtimeHealthArtifact) LoadRuntimeHealth() (RuntimeHealthSnapshot, error) {
	path := filepath.Join(a.dir, runtimeHealthArtifactName)
	info, err := os.Lstat(path)
	if err != nil {
		return RuntimeHealthSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return RuntimeHealthSnapshot{}, fmt.Errorf("runtime health artifact is not a regular file")
	}
	return runtimeHealthFile{path: path}.LoadRuntimeHealth()
}

func NewRuntimeHealthFile(path string) RuntimeHealthSource { return runtimeHealthFile{path: path} }
func (f runtimeHealthFile) LoadRuntimeHealth() (RuntimeHealthSnapshot, error) {
	body, err := os.ReadFile(f.path)
	if err != nil {
		return RuntimeHealthSnapshot{}, err
	}
	var snapshot RuntimeHealthSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return RuntimeHealthSnapshot{}, err
	}
	if snapshot.ObservedAt.IsZero() {
		return RuntimeHealthSnapshot{}, fmt.Errorf("runtime health observed_at required")
	}
	return snapshot, nil
}

type RuntimeHealth struct {
	Freshness Freshness `json:"freshness"`
	Scheduler struct {
		State string `json:"state"`
		Count int    `json:"count"`
	} `json:"scheduler"`
	Heartbeat struct {
		State      string `json:"state"`
		AgeSeconds int64  `json:"age_seconds"`
	} `json:"heartbeat"`
	Lease struct {
		InstanceID string `json:"instance_id,omitempty"`
		Fresh      bool   `json:"fresh"`
	} `json:"lease"`
	DatabaseState string `json:"database_state"`
	ObserverState string `json:"observer_state"`
}

func (s *Service) SetRuntimeHealthSource(source RuntimeHealthSource) { s.health = source }
func (s *Service) RuntimeHealth() (RuntimeHealth, error) {
	out := RuntimeHealth{Freshness: Freshness{State: "unavailable"}, DatabaseState: "unavailable", ObserverState: "unavailable"}
	out.Scheduler.State, out.Heartbeat.State = "unavailable", "unavailable"
	if s.health == nil {
		return out, nil
	}
	snapshot, err := s.health.LoadRuntimeHealth()
	if err != nil {
		return out, nil
	}
	age := s.now().UTC().Sub(snapshot.ObservedAt.UTC())
	out.Freshness.AgeSeconds = int64(age.Seconds())
	out.Scheduler.Count = snapshot.SchedulerCount
	out.Scheduler.State = "unavailable"
	out.Heartbeat.State, out.Heartbeat.AgeSeconds = snapshot.HeartbeatState, snapshot.HeartbeatAgeSeconds
	out.Lease.InstanceID, out.Lease.Fresh = snapshot.LeaseInstanceID, snapshot.LeaseFresh
	out.DatabaseState, out.ObserverState = snapshot.DatabaseState, snapshot.ObserverState
	if age < 0 || age > runtimeHealthFreshFor || snapshot.ObserverState != "pass" {
		out.Freshness.State = "stale"
		return out, nil
	}
	out.Freshness.State = "fresh"
	if snapshot.SchedulerCount == 1 {
		out.Scheduler.State = "healthy"
	} else {
		out.Scheduler.State = "degraded"
	}
	return out, nil
}

type CapitalOverview struct {
	Currency        string   `json:"currency"`
	SourceAt        string   `json:"source_at"`
	AvailableUSDT   float64  `json:"available_usdt"`
	ReservedUSDT    float64  `json:"reserved_usdt"`
	FilledUSDT      float64  `json:"filled_usdt"`
	MaxExposureUSDT float64  `json:"max_exposure_usdt"`
	ProjectionState string   `json:"projection_state"`
	Issues          []string `json:"issues"`
}
type ThesisCapital struct {
	ThesisID        string   `json:"thesis_id"`
	Symbol          string   `json:"symbol"`
	Status          string   `json:"status"`
	MaxExposureUSDT float64  `json:"max_exposure_usdt"`
	ReservedUSDT    float64  `json:"reserved_usdt"`
	FilledUSDT      float64  `json:"filled_usdt"`
	RemainingUSDT   float64  `json:"remaining_usdt"`
	UpdatedAt       string   `json:"updated_at"`
	Blockers        []string `json:"blockers"`
}
type ThesisCapitalPage struct {
	Items []ThesisCapital `json:"items"`
	Limit int             `json:"limit"`
}

func (s *Service) CapitalOverview() (CapitalOverview, error) {
	projections, err := s.db.ThesisCapitalProjectionAudits()
	if err != nil {
		return CapitalOverview{}, fmt.Errorf("read capital projections: %w", err)
	}
	out := CapitalOverview{Currency: "USDT", SourceAt: s.now().UTC().Format(time.RFC3339), Issues: []string{}, ProjectionState: "healthy"}
	for _, projection := range projections {
		ledger := projection.Ledger
		out.MaxExposureUSDT += ledger.MaxExposureUSDT
		out.ReservedUSDT += ledger.ReservedUSDT
		out.FilledUSDT += ledger.FilledUSDT
		out.AvailableUSDT += ledger.RemainingDCAUSDT
		if !projection.Healthy() {
			out.ProjectionState = "drifted"
			out.Issues = append(out.Issues, projection.Mismatches...)
		}
	}
	return out, nil
}

func (s *Service) ThesisCapital(limit int) (ThesisCapitalPage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.ThesisCapitalProjectionAudits()
	if err != nil {
		return ThesisCapitalPage{}, fmt.Errorf("read thesis capital: %w", err)
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := ThesisCapitalPage{Items: make([]ThesisCapital, 0, len(rows)), Limit: limit}
	for _, p := range rows {
		l := p.Ledger
		blockers := append([]string{}, p.Mismatches...)
		if !p.Healthy() {
			blockers = append(blockers, "projection_drift")
		}
		out.Items = append(out.Items, ThesisCapital{ThesisID: l.ThesisID, Symbol: l.Symbol, Status: l.Status, MaxExposureUSDT: l.MaxExposureUSDT, ReservedUSDT: l.ReservedUSDT, FilledUSDT: l.FilledUSDT, RemainingUSDT: l.RemainingDCAUSDT, UpdatedAt: l.UpdatedAt.UTC().Format(time.RFC3339), Blockers: blockers})
	}
	return out, nil
}
