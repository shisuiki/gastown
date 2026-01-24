package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

const mailInjectIdleThreshold = 2 * time.Minute

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

	// Inject mode: only inject if the tmux session has been idle long enough
	if mailCheckInject {
		if !shouldInjectMail() {
			return nil
		}
		if unread > 0 {
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
		}
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

func shouldInjectMail() bool {
	if os.Getenv("TMUX") == "" {
		// Not in tmux; fall back to prior behavior.
		return true
	}

	sessionName, err := currentTmuxSessionName()
	if err != nil || sessionName == "" {
		return false
	}

	idle, err := tmuxSessionIdleDuration(sessionName)
	if err != nil {
		return false
	}

	return idle >= mailInjectIdleThreshold
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
