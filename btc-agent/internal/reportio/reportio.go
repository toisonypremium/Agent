package reportio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0700)
}

func WriteJSON(dir, name string, v any) error {
	if err := EnsureDir(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), b, 0600)
}

func WriteMarkdown(dir, name, text string) error {
	if err := EnsureDir(dir); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(text), 0600)
}

func SafeLabel(label string) string {
	label = strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(label)
	label = strings.TrimSpace(label)
	if label == "" {
		return "telegram"
	}
	return label
}
