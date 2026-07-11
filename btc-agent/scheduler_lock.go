package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func acquireSchedulerProcessLock() (func(), error) {
	path := os.Getenv("BTC_AGENT_SCHEDULER_LOCK_FILE")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("scheduler lock home: %w", err)
		}
		path = filepath.Join(home, ".btc-agent-scheduler.lock")
	}
	if b, err := os.ReadFile(path); err == nil {
		pidText := strings.TrimSpace(string(b))
		if pid, err := strconv.Atoi(pidText); err == nil && pid > 0 {
			if err := syscall.Kill(pid, 0); err == nil {
				return nil, fmt.Errorf("btc-agent scheduler already running pid=%d", pid)
			}
		}
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("write scheduler lock: %w", err)
	}
	return func() {
		b, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(b)) == strconv.Itoa(os.Getpid()) {
			_ = os.Remove(path)
		}
	}, nil
}
