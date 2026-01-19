package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	gracefulTimeout = 2 * time.Second
)

// BdDaemonInfo represents the status of a single bd daemon instance.
type BdDaemonInfo struct {
	Workspace       string `json:"workspace"`
	SocketPath      string `json:"socket_path"`
	PID             int    `json:"pid"`
	Version         string `json:"version"`
	Status          string `json:"status"`
	Issue           string `json:"issue,omitempty"`
	VersionMismatch bool   `json:"version_mismatch,omitempty"`
}

// BdDaemonHealth represents the overall health of bd daemons.
type BdDaemonHealth struct {
	Total        int            `json:"total"`
	Healthy      int            `json:"healthy"`
	Stale        int            `json:"stale"`
	Mismatched   int            `json:"mismatched"`
	Unresponsive int            `json:"unresponsive"`
	Daemons      []BdDaemonInfo `json:"daemons"`
}

// CheckBdDaemonHealth checks the health of all bd daemons.
// Returns nil if no daemons are running (which is fine, bd will use direct mode).
func CheckBdDaemonHealth() (*BdDaemonHealth, error) {
	cmd := exec.Command("bd", "daemon", "health", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// bd daemon health may fail if bd not installed or other issues
		// Return nil to indicate we can't check (not an error for status display)
		return nil, nil
	}

	var health BdDaemonHealth
	if err := json.Unmarshal(stdout.Bytes(), &health); err != nil {
		return nil, fmt.Errorf("parsing daemon health: %w", err)
	}

	return &health, nil
}

// EnsureBdDaemonHealth checks if bd daemons are healthy and attempts to restart if needed.
// Returns a warning message if there were issues, or empty string if everything is fine.
// This is non-blocking - it will not fail if daemons can't be started.
func EnsureBdDaemonHealth(workDir string) string {
	health, err := CheckBdDaemonHealth()
	if err != nil || health == nil {
		// Can't check daemon health - proceed without warning
		return ""
	}

	// No daemons running is fine - bd will use direct mode
	if health.Total == 0 {
		return ""
	}

	// Check if any daemons need attention
	needsRestart := false
	for _, d := range health.Daemons {
		switch d.Status {
		case "healthy":
			// Good
		case "version_mismatch", "stale", "unresponsive":
			needsRestart = true
		}
	}

	if !needsRestart {
		return ""
	}

	// Clean stale lock files for unhealthy daemons before restart
	cleanStaleLocksForUnhealthyDaemons(health)

	// Ensure repository fingerprint exists for unhealthy daemons
	ensureFingerprintForUnhealthyDaemons(health)

	// Attempt to restart daemons
	if restartErr := restartBdDaemons(health); restartErr != nil {
		return fmt.Sprintf("bd daemons unhealthy (restart failed: %v)", restartErr)
	}

	// Verify restart worked
	time.Sleep(500 * time.Millisecond)
	newHealth, err := CheckBdDaemonHealth()
	if err != nil || newHealth == nil {
		return "bd daemons restarted but status unknown"
	}

	if newHealth.Healthy < newHealth.Total {
		return fmt.Sprintf("bd daemons partially healthy (%d/%d)", newHealth.Healthy, newHealth.Total)
	}

	return "" // Successfully restarted
}

// cleanStaleLocksForUnhealthyDaemons cleans stale lock files for unhealthy bd daemons.
func cleanStaleLocksForUnhealthyDaemons(health *BdDaemonHealth) {
	for _, d := range health.Daemons {
		if d.Status != "healthy" && d.Workspace != "" {
			cleaned, err := CleanStaleBdLockFiles(d.Workspace)
			if err != nil {
				// Log error? No logger available; ignore for now.
				continue
			}
			if cleaned > 0 {
				// Could log but no logger
			}
		}
	}
}

// ensureFingerprintForUnhealthyDaemons ensures repository fingerprint exists for unhealthy daemons.
func ensureFingerprintForUnhealthyDaemons(health *BdDaemonHealth) {
	for _, d := range health.Daemons {
		if d.Status != "healthy" && d.Workspace != "" {
			_ = ensureFingerprint(d.Workspace)
		}
	}
}

// ensureFingerprint ensures the repository fingerprint exists in the beads database.
// This is idempotent - safe on both new and legacy (pre-0.17.5) databases.
// Without fingerprint, the bd daemon fails to start silently.
func ensureFingerprint(workspace string) error {
	cmd := exec.Command("bd", "migrate", "--update-repo-id", "--yes")
	cmd.Dir = workspace
	// Ignore errors - fingerprint is optional for functionality
	_ = cmd.Run()
	return nil
}

// restartBdDaemons restarts all bd daemons.
func restartBdDaemons(health *BdDaemonHealth) error {
	// Try graceful stop for each unhealthy daemon first
	for _, d := range health.Daemons {
		if d.Status != "healthy" && d.Workspace != "" {
			// Attempt bd daemon stop for this workspace
			cmd := exec.Command("bd", "daemon", "stop")
			cmd.Dir = d.Workspace
			_ = cmd.Run() // Ignore errors - fallback to pkill
		}
	}

	// Stop all daemons using pkill to ensure cleanup
	_ = exec.Command("pkill", "-TERM", "-f", "bd daemon").Run()

	// Give time for cleanup
	time.Sleep(200 * time.Millisecond)

	// Clean stale lock files for all daemons (including healthy ones that were killed)
	for _, d := range health.Daemons {
		if d.Workspace != "" {
			_, _ = CleanStaleBdLockFiles(d.Workspace) // Ignore errors
		}
	}

	// Start daemons for known locations
	// The daemon will auto-start when bd commands are run in those directories
	// Just running any bd command will trigger daemon startup if configured
	return nil
}

// StartBdDaemonIfNeeded starts the bd daemon for a specific workspace if not running.
// This is a best-effort operation - failures are logged but don't block execution.
func StartBdDaemonIfNeeded(workDir string) error {
	cmd := exec.Command("bd", "daemon", "start")
	cmd.Dir = workDir
	return cmd.Run()
}

// StopAllBdProcesses stops all bd daemon and activity processes.
// Returns (daemonsKilled, activityKilled, error).
// If dryRun is true, returns counts without stopping anything.
func StopAllBdProcesses(dryRun, force bool) (int, int, error) {
	if _, err := exec.LookPath("bd"); err != nil {
		return 0, 0, nil
	}

	daemonsBefore := CountBdDaemons()
	activityBefore := CountBdActivityProcesses()

	if dryRun {
		return daemonsBefore, activityBefore, nil
	}

	daemonsKilled, daemonsRemaining := stopBdDaemons(force)
	activityKilled, activityRemaining := stopBdActivityProcesses(force)

	if daemonsRemaining > 0 {
		return daemonsKilled, activityKilled, fmt.Errorf("bd daemon shutdown incomplete: %d still running", daemonsRemaining)
	}
	if activityRemaining > 0 {
		return daemonsKilled, activityKilled, fmt.Errorf("bd activity shutdown incomplete: %d still running", activityRemaining)
	}

	return daemonsKilled, activityKilled, nil
}

// CountBdDaemons returns count of running bd daemons.
// Uses pgrep instead of "bd daemon list" to avoid triggering daemon auto-start
// during shutdown verification.
func CountBdDaemons() int {
	// Use pgrep -f with wc -l for cross-platform compatibility
	// (macOS pgrep doesn't support -c flag)
	cmd := exec.Command("sh", "-c", "pgrep -f 'bd daemon' 2>/dev/null | wc -l")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return count
}


func stopBdDaemons(force bool) (int, int) {
	before := CountBdDaemons()
	if before == 0 {
		return 0, 0
	}

	// Use pkill directly instead of "bd daemon killall" to avoid triggering
	// daemon auto-start as a side effect of running bd commands.
	// Note: pkill -f pattern may match unintended processes in rare cases
	// (e.g., editors with "bd daemon" in file content). This is acceptable
	// given the alternative of respawning daemons during shutdown.
	if force {
		_ = exec.Command("pkill", "-9", "-f", "bd daemon").Run()
	} else {
		_ = exec.Command("pkill", "-TERM", "-f", "bd daemon").Run()
		time.Sleep(gracefulTimeout)
		if remaining := CountBdDaemons(); remaining > 0 {
			_ = exec.Command("pkill", "-9", "-f", "bd daemon").Run()
		}
	}

	time.Sleep(100 * time.Millisecond)

	final := CountBdDaemons()
	killed := before - final
	if killed < 0 {
		killed = 0 // Race condition: more processes spawned than we killed
	}
	return killed, final
}

// CountBdActivityProcesses returns count of running `bd activity` processes.
func CountBdActivityProcesses() int {
	// Use pgrep -f with wc -l for cross-platform compatibility
	// (macOS pgrep doesn't support -c flag)
	cmd := exec.Command("sh", "-c", "pgrep -f 'bd activity' 2>/dev/null | wc -l")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return count
}

func stopBdActivityProcesses(force bool) (int, int) {
	before := CountBdActivityProcesses()
	if before == 0 {
		return 0, 0
	}

	if force {
		_ = exec.Command("pkill", "-9", "-f", "bd activity").Run()
	} else {
		_ = exec.Command("pkill", "-TERM", "-f", "bd activity").Run()
		time.Sleep(gracefulTimeout)
		if remaining := CountBdActivityProcesses(); remaining > 0 {
			_ = exec.Command("pkill", "-9", "-f", "bd activity").Run()
		}
	}

	time.Sleep(100 * time.Millisecond)

	after := CountBdActivityProcesses()
	killed := before - after
	if killed < 0 {
		killed = 0 // Race condition: more processes spawned than we killed
	}
	return killed, after
}
