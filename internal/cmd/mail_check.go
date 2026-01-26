package cmd

import (
	"context"
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
const mailInjectMarkerPrefix = "mail_inject_pending"
const mailInjectTmuxTimeout = 2 * time.Second
const mailInjectSubjectLimit = 8

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
			clearMailInjectMarkers(workDir, mailCheckSession, address)
			return nil
		}
		sessionName, ok := canInjectMail(mailCheckSession)
		if !ok {
			scheduleMailInjectRetry(workDir, sessionName, address)
			return nil
		}
		// Get subjects for context
		messages, _ := mailbox.ListUnread()
		reminderStdout, reminderNudge := buildMailReminder(unread, messages)

		if mailCheckNudge {
			if sessionName == "" {
				sessionName = mailCheckSession
			}
			if sessionName != "" {
				if err := tmux.NewTmux().NudgeSession(sessionName, reminderNudge); err != nil {
					scheduleMailInjectRetry(workDir, sessionName, address)
					return nil
				}
				clearMailInjectMarkers(workDir, sessionName, address)
				return nil
			}
			fmt.Print(reminderStdout)
			clearMailInjectMarkers(workDir, sessionName, address)
			return nil
		}

		fmt.Print(reminderStdout)
		clearMailInjectMarkers(workDir, sessionName, address)
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

func canInjectMail(sessionOverride string) (string, bool) {
	sessionName := sessionOverride
	if sessionName == "" {
		if os.Getenv("TMUX") == "" {
			// Not in tmux; fall back to prior behavior.
			return "", true
		}

		var err error
		sessionName, err = currentTmuxSessionName()
		if err != nil || sessionName == "" {
			return "", false
		}
	}

	idle, err := tmuxSessionIdleDuration(sessionName)
	if err != nil {
		return sessionName, false
	}

	return sessionName, idle >= mailInjectIdleThreshold
}

func currentTmuxSessionName() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mailInjectTmuxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#S")
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("tmux session lookup timed out")
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func tmuxSessionIdleDuration(session string) (time.Duration, error) {
	if session == "" {
		return 0, fmt.Errorf("empty session name")
	}

	ctx, cancel := context.WithTimeout(context.Background(), mailInjectTmuxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}|#{session_activity}|#{session_last_attached}",
		"-f", fmt.Sprintf("#{==:#{session_name},%s}", session))
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return 0, fmt.Errorf("tmux session activity lookup timed out")
	}
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

	activity := ""
	if len(parts) > 1 {
		activity = parts[1]
	}
	lastAttached := ""
	if len(parts) > 2 {
		lastAttached = parts[2]
	}

	if activity != "" {
		if ts, err := parseTmuxTimestamp(activity); err == nil {
			return time.Since(ts), nil
		}
	}

	if lastAttached != "" {
		if ts, err := parseTmuxTimestamp(lastAttached); err == nil {
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
	Identity    string `json:"identity,omitempty"`
}

func scheduleMailInjectRetry(workDir, session, identity string) {
	markerPath := mailInjectMarkerPath(workDir, session, identity)
	if shouldSkipMailInjectSchedule(markerPath) {
		return
	}

	nextAttempt := time.Now().UTC().Add(mailInjectIdleThreshold)
	marker := mailInjectMarker{
		NextAttempt: nextAttempt.Format(time.RFC3339),
		Session:     session,
		Identity:    identity,
	}
	if err := writeMailInjectMarker(markerPath, marker); err != nil {
		return
	}

	gtPath, err := exec.LookPath("gt")
	if err != nil {
		gtPath = "gt"
	}
	command := fmt.Sprintf("sleep %d; cd %s && %s mail check --inject --nudge",
		int(mailInjectIdleThreshold.Seconds()),
		shellQuote(workDir),
		shellQuote(gtPath),
	)
	if identity != "" {
		command += fmt.Sprintf(" --identity %s", shellQuote(identity))
	}
	if session != "" {
		command += fmt.Sprintf(" --session %s", shellQuote(session))
	}

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

func clearMailInjectMarkers(workDir, session, identity string) {
	if session != "" {
		_ = os.Remove(mailInjectMarkerPath(workDir, session, ""))
	}
	if identity != "" {
		_ = os.Remove(mailInjectMarkerPath(workDir, "", identity))
	}
}

func mailInjectMarkerKey(session, identity string) string {
	key := session
	if key == "" {
		key = identity
	}
	if key == "" {
		key = "unknown"
	}
	replacer := strings.NewReplacer("/", "_", ":", "_")
	return replacer.Replace(key)
}

func mailInjectMarkerPath(workDir, session, identity string) string {
	key := mailInjectMarkerKey(session, identity)
	filename := fmt.Sprintf("%s_%s.json", mailInjectMarkerPrefix, key)
	return filepath.Join(workDir, constants.DirRuntime, filename)
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

func buildMailReminder(unread int, messages []*mail.Message) (string, string) {
	var subjects []string
	if len(messages) > 0 {
		limit := mailInjectSubjectLimit
		if limit <= 0 {
			limit = len(messages)
		}
		for i, msg := range messages {
			if i >= limit {
				break
			}
			subjects = append(subjects, fmt.Sprintf("- %s from %s: %s", msg.ID, msg.From, msg.Subject))
		}
		if extra := len(messages) - len(subjects); extra > 0 {
			subjects = append(subjects, fmt.Sprintf("... (+%d more)", extra))
		}
	}

	var stdoutBuilder strings.Builder
	stdoutBuilder.WriteString("<system-reminder>\n")
	stdoutBuilder.WriteString(fmt.Sprintf("You have %d unread message(s) in your inbox.\n", unread))
	if len(subjects) > 0 {
		stdoutBuilder.WriteString("\n")
		stdoutBuilder.WriteString(strings.Join(subjects, "\n"))
		stdoutBuilder.WriteString("\n")
	}
	stdoutBuilder.WriteString("\nRun 'gt mail inbox' to see your messages, or 'gt mail read <id>' for a specific message.\n")
	stdoutBuilder.WriteString("</system-reminder>\n")

	var nudgeBuilder strings.Builder
	nudgeBuilder.WriteString(fmt.Sprintf("📬 You have %d unread message(s) in your inbox.", unread))
	if len(subjects) > 0 {
		nudgeBuilder.WriteString("\n")
		nudgeBuilder.WriteString(strings.Join(subjects, "\n"))
	}
	nudgeBuilder.WriteString("\nRun 'gt mail inbox' to see your messages, or 'gt mail read <id>' for a specific message.")

	return stdoutBuilder.String(), nudgeBuilder.String()
}
