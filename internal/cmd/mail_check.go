package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

const mailInjectIdleThreshold = 2 * time.Minute
const mailInjectMarkerName = "mail_inject_pending.json"

func runMailCheck(cmd *cobra.Command, args []string) error {
	// Determine which inbox (priority: --identity flag, auto-detect)
	address := ""
	if mailCheckIdentity != "" {
		address = mailCheckIdentity
	} else {
		address = detectSender()
	}

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		if mailCheckInject {
			// Inject mode: always exit 0, silent on error
			return nil
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		if mailCheckInject {
			return nil
		}
		return fmt.Errorf("getting mailbox: %w", err)
	}

	// Count unread
	_, unread, err := mailbox.Count()
	if err != nil {
		if mailCheckInject {
			return nil
		}
		return fmt.Errorf("counting messages: %w", err)
	}

	// JSON output
	if mailCheckJSON {
		result := map[string]interface{}{
			"address": address,
			"unread":  unread,
			"has_new": unread > 0,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Inject mode: delay until the tmux session has been idle long enough
	if mailCheckInject {
		if unread == 0 {
			clearMailInjectMarker(workDir)
			return nil
		}
		sessionName, ok := canInjectMail()
		if !ok {
			scheduleMailInjectRetry(workDir, sessionName)
			return nil
		}
		clearMailInjectMarker(workDir)
		// Get subjects for context
		messages, _ := mailbox.ListUnread()
		var subjects []string
		for _, msg := range messages {
			subjects = append(subjects, fmt.Sprintf("- %s from %s: %s", msg.ID, msg.From, msg.Subject))
		}

		fmt.Println("<system-reminder>")
		fmt.Printf("You have %d unread message(s) in your inbox.\n\n", unread)
		for _, s := range subjects {
			fmt.Println(s)
		}
		fmt.Println()
		fmt.Println("Run 'gt mail inbox' to see your messages, or 'gt mail read <id>' for a specific message.")
		fmt.Println("</system-reminder>")
		return nil
	}

	// Normal mode
	if unread > 0 {
		fmt.Printf("%s %d unread message(s)\n", style.Bold.Render("📬"), unread)
		return NewSilentExit(0)
	}
	fmt.Println("No new mail")
	return NewSilentExit(1)
}

func canInjectMail() (string, bool) {
	if os.Getenv("TMUX") == "" {
		// Not in tmux; fall back to prior behavior.
		return "", true
	}

	sessionName, err := currentTmuxSessionName()
	if err != nil || sessionName == "" {
		return "", false
	}

	idle, err := tmuxSessionIdleDuration(sessionName)
	if err != nil {
		return sessionName, false
	}

	return sessionName, idle >= mailInjectIdleThreshold
}

func currentTmuxSessionName() (string, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func tmuxSessionIdleDuration(session string) (time.Duration, error) {
	info, err := tmux.NewTmux().GetSessionInfo(session)
	if err != nil {
		return 0, err
	}

	if info.Activity != "" {
		if ts, err := parseTmuxTimestamp(info.Activity); err == nil {
			return time.Since(ts), nil
		}
	}

	if info.LastAttached != "" {
		if ts, err := parseTmuxTimestamp(info.LastAttached); err == nil {
			return time.Since(ts), nil
		}
	}

	return 0, fmt.Errorf("no session activity timestamp")
}

func parseTmuxTimestamp(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	epoch, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(epoch, 0), nil
}

type mailInjectMarker struct {
	NextAttempt string `json:"next_attempt"`
	Session     string `json:"session,omitempty"`
}

func scheduleMailInjectRetry(workDir, session string) {
	markerPath := mailInjectMarkerPath(workDir)
	if shouldSkipMailInjectSchedule(markerPath) {
		return
	}

	nextAttempt := time.Now().UTC().Add(mailInjectIdleThreshold)
	marker := mailInjectMarker{
		NextAttempt: nextAttempt.Format(time.RFC3339),
		Session:     session,
	}
	if err := writeMailInjectMarker(markerPath, marker); err != nil {
		return
	}

	gtPath, err := exec.LookPath("gt")
	if err != nil {
		gtPath = "gt"
	}
	command := fmt.Sprintf("sleep %d; cd %s && %s mail check --inject",
		int(mailInjectIdleThreshold.Seconds()),
		shellQuote(workDir),
		shellQuote(gtPath),
	)

	args := []string{"run-shell"}
	if session != "" {
		args = append(args, "-t", session)
	}
	args = append(args, command)
	cmd := exec.Command("tmux", args...)
	_ = cmd.Run()
}

func shouldSkipMailInjectSchedule(markerPath string) bool {
	marker, err := readMailInjectMarker(markerPath)
	if err != nil {
		return false
	}
	if marker.NextAttempt == "" {
		return false
	}
	next, err := time.Parse(time.RFC3339, marker.NextAttempt)
	if err != nil {
		return false
	}
	return time.Now().UTC().Before(next)
}

func clearMailInjectMarker(workDir string) {
	markerPath := mailInjectMarkerPath(workDir)
	_ = os.Remove(markerPath)
}

func mailInjectMarkerPath(workDir string) string {
	return filepath.Join(workDir, constants.DirRuntime, mailInjectMarkerName)
}

func readMailInjectMarker(path string) (mailInjectMarker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return mailInjectMarker{}, err
	}
	var marker mailInjectMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return mailInjectMarker{}, err
	}
	return marker, nil
}

func writeMailInjectMarker(path string, marker mailInjectMarker) error {
	runtimeDir := filepath.Dir(path)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
