package reportio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultDirPerm  = 0700
	DefaultFilePerm = 0600
)

func EnsureDir(dir string) error {
	return os.MkdirAll(dir, DefaultDirPerm)
}

// WriteJSON marshals v to indented JSON and atomically writes it to dir/name.
func WriteJSON(dir, name string, v any) error {
	if err := EnsureDir(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, name), b, DefaultFilePerm)
}

// WriteMarkdown atomically writes text to dir/name.
func WriteMarkdown(dir, name, text string) error {
	if err := EnsureDir(dir); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, name), []byte(text), DefaultFilePerm)
}

// ReadJSON reads and decodes a JSON file into v. Returns os.ErrNotExist when the file is absent.
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return os.ErrNotExist
	}
	return json.Unmarshal(b, v)
}

// atomicWrite writes data to path via temp-file + rename for crash safety.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func SafeLabel(label string) string {
	label = strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(label)
	label = strings.TrimSpace(label)
	if label == "" {
		return "telegram"
	}
	return label
}
