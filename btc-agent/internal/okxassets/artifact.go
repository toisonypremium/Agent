package okxassets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ArtifactFilename = "web_console_okx_assets.json"
const (
	StateVerified    = "da_xac_minh"
	StateStale       = "du_lieu_cu"
	StateUnavailable = "khong_kha_dung"
)

type Artifact struct {
	SchemaVersion int      `json:"schema_version"`
	Source        string   `json:"nguon"`
	ObservedAt    string   `json:"thoi_diem_quan_sat"`
	State         string   `json:"trang_thai"`
	Assets        []Asset  `json:"tai_san"`
	Warnings      []string `json:"canh_bao"`
}

func WriteArtifact(dir string, s Snapshot, now time.Time) error {
	if s.Source != SourceOKXSpotReadOnly {
		return fmt.Errorf("unexpected artifact source")
	}
	a := Artifact{SchemaVersion: 1, Source: s.Source, ObservedAt: now.UTC().Format(time.RFC3339), State: StateVerified, Assets: s.Assets, Warnings: []string{}}
	body, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".okx-assets-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(body, '\n'))
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, ArtifactFilename))
}
func LoadArtifact(dir string, now time.Time, maxAge time.Duration) (Artifact, error) {
	path := filepath.Join(dir, ArtifactFilename)
	info, err := os.Lstat(path)
	if err != nil {
		return Artifact{State: StateUnavailable}, fmt.Errorf("OKX asset artifact unavailable")
	}
	if !info.Mode().IsRegular() {
		return Artifact{State: StateUnavailable}, fmt.Errorf("OKX asset artifact is not regular")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Artifact{State: StateUnavailable}, err
	}
	var a Artifact
	if err = json.Unmarshal(body, &a); err != nil {
		return Artifact{State: StateUnavailable}, fmt.Errorf("OKX asset artifact malformed")
	}
	at, err := time.Parse(time.RFC3339, a.ObservedAt)
	if err != nil || a.SchemaVersion != 1 || a.Source != SourceOKXSpotReadOnly {
		return Artifact{State: StateUnavailable}, fmt.Errorf("OKX asset artifact invalid")
	}
	if now.Sub(at) > maxAge {
		a.State = StateStale
		return a, nil
	}
	if a.State != StateVerified {
		return Artifact{State: StateUnavailable}, fmt.Errorf("OKX asset artifact state invalid")
	}
	return a, nil
}
