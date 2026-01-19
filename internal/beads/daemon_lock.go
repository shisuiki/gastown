// Package beads provides bd daemon lock file management.
package beads

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// CleanStaleBdLockFiles removes stale bd daemon lock files from a .beads directory.
// A lock file is considered stale if the PID in daemon.pid is dead.
// Returns the number of lock files cleaned.
// This is safe to call even if the daemon is running - it only removes locks
// for dead processes.
func CleanStaleBdLockFiles(beadsDir string) (int, error) {
	// Check if beads directory exists
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return 0, nil // Nothing to clean
	}

	cleaned := 0

	// Check daemon.pid first - if PID is alive, daemon is running and we should
	// not remove any lock files.
	pidFile := filepath.Join(beadsDir, "daemon.pid")
	if pid, err := readPidFile(pidFile); err == nil {
		if processExists(pid) {
			// Daemon is running - do not clean any lock files
			return 0, nil
		}
		// PID is dead - remove daemon.pid
		if err := os.Remove(pidFile); err == nil {
			cleaned++
		}
	}

	// List of lock files to check
	lockFiles := []string{
		"daemon.lock",
		"bd.sock.startlock",
		// bd.sock is a socket file, not a lock - don't remove if daemon might be alive
		// but if PID is dead, socket is stale and safe to remove
		"bd.sock",
	}

	for _, lockFile := range lockFiles {
		fullPath := filepath.Join(beadsDir, lockFile)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		// For lock files, we can't easily check ownership.
		// Since we already verified the daemon PID is dead (or missing),
		// it's safe to remove the lock files.
		if err := os.Remove(fullPath); err == nil {
			cleaned++
		}
	}

	// Also clean daemon.log if it's empty or small? We'll leave it for debugging.
	// But if PID is dead, we could rotate or truncate? Not needed.

	return cleaned, nil
}

// readPidFile reads and parses a PID file.
// Returns 0 if file doesn't exist or contains invalid data.
func readPidFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid PID in %s: %s", path, pidStr)
	}

	return pid, nil
}

// processExists checks if a process with the given PID exists and is alive.
func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Send signal 0 to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}