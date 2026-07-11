package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

type LatestReport struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Analysis    agent1.MarketAnalysis `json:"analysis"`
	Plan        agent2.Plan           `json:"plan"`
}

func SaveReportFiles(dir string, analysis agent1.MarketAnalysis, plan agent2.Plan, markdown string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "latest.md"), []byte(markdown), 0600); err != nil {
		return err
	}
	payload := LatestReport{GeneratedAt: time.Now(), Analysis: analysis, Plan: plan}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "latest.json"), b, 0600)
}

type reportFileInfo struct {
	name    string
	path    string
	modTime time.Time
}

func PruneReportFiles(dir string, maxFiles int, protected []string) (int, error) {
	if maxFiles <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	protectedSet := map[string]bool{}
	for _, name := range protected {
		protectedSet[name] = true
	}
	files := []reportFileInfo{}
	candidates := []reportFileInfo{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		file := reportFileInfo{name: entry.Name(), path: filepath.Join(dir, entry.Name()), modTime: info.ModTime()}
		files = append(files, file)
		if !isProtectedReportFile(entry.Name(), protectedSet) {
			candidates = append(candidates, file)
		}
	}
	deleteTarget := len(files) - maxFiles
	if deleteTarget <= 0 {
		return 0, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].name < candidates[j].name
		}
		return candidates[i].modTime.Before(candidates[j].modTime)
	})
	deleted := 0
	for _, file := range candidates {
		if deleted >= deleteTarget {
			break
		}
		if err := os.Remove(file.path); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func isProtectedReportFile(name string, protected map[string]bool) bool {
	if protected[name] {
		return true
	}
	if strings.HasPrefix(name, "live_") && strings.Contains(name, "_latest.") {
		return true
	}
	return false
}
