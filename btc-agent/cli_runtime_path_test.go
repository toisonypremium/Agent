package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalConfigPathIndependentOfWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(app, "config.yaml")
	if err := os.WriteFile(configPath, []byte("app: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := canonicalConfigPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("canonical path = %q, want %q", got, want)
	}
}

func TestCanonicalConfigPathResolvesSymlink(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0700); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(app, "config.yaml")
	if err := os.WriteFile(realPath, []byte("app: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "config-link.yaml")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}
	got, err := canonicalConfigPath(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != realPath {
		t.Fatalf("canonical path = %q, want %q", got, realPath)
	}
}

func TestCanonicalConfigPathRejectsDirectory(t *testing.T) {
	_, err := canonicalConfigPath(t.TempDir())
	if err == nil {
		t.Fatal("expected directory config path to fail")
	}
}
