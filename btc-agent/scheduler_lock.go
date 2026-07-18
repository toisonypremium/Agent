package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open scheduler lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("btc-agent scheduler already running (lock held)")
		}
		return nil, fmt.Errorf("acquire scheduler lock: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("truncate scheduler lock: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("seek scheduler lock: %w", err)
	}
	if _, err := file.WriteString(strconv.Itoa(os.Getpid()) + "\n"); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("write scheduler lock: %w", err)
	}
	return func() {
		_ = file.Truncate(0)
		_, _ = file.Seek(0, 0)
		_ = file.Sync()
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		// Keep the path or remove it; an open lock descriptor, not the path,
		// is the authority. Removing is only cosmetic and avoids stale files.
		_ = os.Remove(path)
	}, nil
}
