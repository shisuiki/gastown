package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runMailSend(cmd *cobra.Command, args []string) error {
	var to string

	if mailSendSelf {
		// Auto-detect identity from cwd
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		townRoot, err := workspace.FindFromCwd()
		if err != nil || townRoot == "" {
			return fmt.Errorf("not in a Gas Town workspace")
		}
		roleInfo, err := GetRoleWithContext(cwd, townRoot)
		if err != nil {
			return fmt.Errorf("detecting role: %w", err)
		}
		ctx := RoleContext{
			Role:     roleInfo.Role,
			Rig:      roleInfo.Rig,
			Polecat:  roleInfo.Polecat,
			TownRoot: townRoot,
			WorkDir:  cwd,
		}
		to = buildAgentIdentity(ctx)
		if to == "" {
			return fmt.Errorf("cannot determine identity (role: %s)", ctx.Role)
		}
	} else if len(args) > 0 {
		to = args[0]
	} else {
		return fmt.Errorf("address required (or use --self)")
	}

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Determine sender
	from := detectSender()

	// Create message
	msg := &mail.Message{
		From:    from,
		To:      to,
		Subject: mailSubject,
		Body:    mailBody,
	}

	// Set priority (--urgent overrides --priority)
	if mailUrgent {
		msg.Priority = mail.PriorityUrgent
	} else {
		msg.Priority = mail.PriorityFromInt(mailPriority)
	}

	// Resolve effective notify flag (--no-notify overrides --notify default)
	shouldNotify := mailNotify && !mailNoNotify

	if shouldNotify && msg.Priority == mail.PriorityNormal {
		msg.Priority = mail.PriorityHigh
	}

	// Set message type
	msg.Type = mail.ParseMessageType(mailType)

	// Set pinned flag
	msg.Pinned = mailPinned

	// Set wisp flag (ephemeral message) - default true, --permanent overrides
	msg.Wisp = mailWisp && !mailPermanent

	// Set CC recipients
	msg.CC = mailCC

	// Handle reply-to: auto-set type to reply and look up thread
	if mailReplyTo != "" {
		msg.ReplyTo = mailReplyTo
		if msg.Type == mail.TypeNotification {
			msg.Type = mail.TypeReply
		}

		// Look up original message to get thread ID
		router := mail.NewRouter(workDir)
		mailbox, err := router.GetMailbox(from)
		if err == nil {
			if original, err := mailbox.Get(mailReplyTo); err == nil {
				msg.ThreadID = original.ThreadID
			}
		}
	}

	// Generate thread ID for new threads
	if msg.ThreadID == "" {
		msg.ThreadID = generateThreadID()
	}

	// Use address resolver for new address types
	townRoot, _ := workspace.FindFromCwd()
	b := beads.New(townRoot)
	resolver := mail.NewResolver(b, townRoot)

	recipients, err := resolver.Resolve(to)
	if err != nil {
		// Fall back to legacy routing if resolver fails
		router := mail.NewRouter(workDir)
		if err := router.Send(msg); err != nil {
			return fmt.Errorf("sending message: %w", err)
		}
		_ = events.LogFeed(events.TypeMail, from, events.MailPayload(to, mailSubject))
		fmt.Printf("%s Message sent to %s\n", style.Bold.Render("✓"), to)
		fmt.Printf("  Subject: %s\n", mailSubject)

		// Send notification (enabled by default, use --no-notify to disable)
		if shouldNotify {
			notifyMailRecipients(townRoot, []string{to}, from, mailSubject)
		}
		return nil
	}

	// Route based on recipient type
	router := mail.NewRouter(workDir)
	var recipientAddrs []string

	for _, rec := range recipients {
		switch rec.Type {
		case mail.RecipientQueue:
			// Queue messages: single message, workers claim
			msg.To = rec.Address
			if err := router.Send(msg); err != nil {
				return fmt.Errorf("sending to queue: %w", err)
			}
			recipientAddrs = append(recipientAddrs, rec.Address)

		case mail.RecipientChannel:
			// Channel messages: single message, broadcast
			msg.To = rec.Address
			if err := router.Send(msg); err != nil {
				return fmt.Errorf("sending to channel: %w", err)
			}
			recipientAddrs = append(recipientAddrs, rec.Address)

		default:
			// Direct/agent messages: fan out to each recipient
			msgCopy := *msg
			msgCopy.To = rec.Address
			if err := router.Send(&msgCopy); err != nil {
				return fmt.Errorf("sending to %s: %w", rec.Address, err)
			}
			recipientAddrs = append(recipientAddrs, rec.Address)
		}
	}

	// Log mail event to activity feed
	_ = events.LogFeed(events.TypeMail, from, events.MailPayload(to, mailSubject))

	fmt.Printf("%s Message sent to %s\n", style.Bold.Render("✓"), to)
	fmt.Printf("  Subject: %s\n", mailSubject)

	// Show resolved recipients if fan-out occurred
	if len(recipientAddrs) > 1 || (len(recipientAddrs) == 1 && recipientAddrs[0] != to) {
		fmt.Printf("  Recipients: %s\n", strings.Join(recipientAddrs, ", "))
	}

	if len(msg.CC) > 0 {
		fmt.Printf("  CC: %s\n", strings.Join(msg.CC, ", "))
	}
	if msg.Type != mail.TypeNotification {
		fmt.Printf("  Type: %s\n", msg.Type)
	}

	// Send notification (enabled by default, use --no-notify to disable)
	if shouldNotify {
		notifyMailRecipients(townRoot, recipientAddrs, from, mailSubject)
	}

	return nil
}

// generateThreadID creates a random thread ID for new message threads.
func generateThreadID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b) // crypto/rand.Read only fails on broken system
	return "thread-" + hex.EncodeToString(b)
}

// notifyMailRecipients sends tmux notifications to mail recipients.
// This is called when --notify flag is set on mail send.
// Errors are logged but don't fail the mail send operation.
func notifyMailRecipients(townRoot string, recipients []string, from, subject string) {
	t := tmux.NewTmux()
	message := fmt.Sprintf("[mail from %s] %s", from, subject)

	var notified int
	for _, addr := range recipients {
		sessionName := resolveMailAddrToSession(addr)
		if sessionName == "" {
			continue
		}

		// Check if session exists
		exists, err := t.HasSession(sessionName)
		if err != nil || !exists {
			continue
		}

		// Check DND status (fail-open: nudge if we can't check)
		if townRoot != "" {
			shouldSend, _, _ := shouldNudgeTarget(townRoot, addr, false)
			if !shouldSend {
				fmt.Printf("  %s %s (DND enabled)\n", style.Dim.Render("○"), addr)
				continue
			}
		}

		// Send the nudge
		if err := t.NudgeSession(sessionName, message); err != nil {
			fmt.Printf("  %s Failed to notify %s: %v\n", style.Dim.Render("○"), addr, err)
			continue
		}

		notified++
	}

	if notified > 0 {
		fmt.Printf("  %s Notified %d recipient(s)\n", style.Bold.Render("📬"), notified)
	}
}

// resolveMailAddrToSession converts a mail address to a tmux session name.
// Returns empty string if the address cannot be resolved.
func resolveMailAddrToSession(addr string) string {
	// Handle special cases
	switch addr {
	case "mayor", "mayor/":
		return session.MayorSessionName()
	case "deacon", "deacon/":
		return session.DeaconSessionName()
	}

	// Parse rig/role format
	if !strings.Contains(addr, "/") {
		return ""
	}

	parts := strings.SplitN(addr, "/", 2)
	if len(parts) != 2 {
		return ""
	}

	rig := parts[0]
	role := parts[1]

	// Handle trailing slash (e.g., "gastown/witness/")
	role = strings.TrimSuffix(role, "/")

	switch role {
	case "witness":
		return session.WitnessSessionName(rig)
	case "refinery":
		return session.RefinerySessionName(rig)
	default:
		// Check for crew format: crew/<name>
		if strings.HasPrefix(role, "crew/") {
			crewName := strings.TrimPrefix(role, "crew/")
			return crewSessionName(rig, crewName)
		}
		// Assume polecat
		return fmt.Sprintf("gt-%s-%s", rig, role)
	}
}
