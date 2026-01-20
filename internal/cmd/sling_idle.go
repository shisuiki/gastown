package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func sessionIdleDuration(sessionName string) (time.Duration, error) {
	if sessionName == "" {
		return 0, fmt.Errorf("empty session name")
	}

	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}",
		"-f", fmt.Sprintf("#{==:#{session_name},%s}", sessionName))
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return 0, fmt.Errorf("session not found")
	}

	line := output
	if idx := strings.Index(output, "\n"); idx >= 0 {
		line = output[:idx]
	}

	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected session activity format")
	}

	var activityUnix int64
	if _, err := fmt.Sscanf(parts[1], "%d", &activityUnix); err != nil || activityUnix == 0 {
		return 0, fmt.Errorf("invalid session activity timestamp")
	}

	idleFor := time.Since(time.Unix(activityUnix, 0))
	if idleFor < 0 {
		idleFor = 0
	}

	return idleFor, nil
}
