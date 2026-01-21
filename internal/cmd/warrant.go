package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var warrantCmd = &cobra.Command{
	Use:     "warrant",
	Aliases: []string{"warrants"},
	GroupID: GroupAgents,
	Short:   "File death warrants for agent cleanup",
	RunE:    requireSubcommand,
	Long: `File and inspect death warrants for stuck or abandoned agents.

Warrants are tracked in beads so the Boot shutdown dance can audit, investigate,
and execute termination safely.`,
}

var warrantFileCmd = &cobra.Command{
	Use:   "file <target>",
	Short: "File a death warrant",
	Long: `File a death warrant for a target agent or session.

Examples:
  gt warrant file gastown/polecats/nux --reason "Zombie detected: no session, no hook, idle >10m"
  gt warrant file deacon/dogs/alpha --reason "Stuck: working on cleanup for 2h"
  gt warrant file gt-gastown-nux --reason "Unresponsive session"
  gt warrant file gastown/witness --reason "Unresponsive health checks"`,
	Args: cobra.ExactArgs(1),
	RunE: runWarrantFile,
}

var (
	warrantFileReason    string
	warrantFileRequester string
	warrantFileSession   string
	warrantFileDryRun    bool
	warrantFileJSON      bool
)

type warrantPayload struct {
	ID        string `json:"id,omitempty"`
	Title     string `json:"title"`
	Target    string `json:"target"`
	Session   string `json:"session,omitempty"`
	Reason    string `json:"reason"`
	Requester string `json:"requester"`
	FiledAt   string `json:"filed_at"`
	DryRun    bool   `json:"dry_run"`
}

func init() {
	warrantCmd.AddCommand(warrantFileCmd)
	rootCmd.AddCommand(warrantCmd)

	warrantFileCmd.Flags().StringVar(&warrantFileReason, "reason", "", "Why the warrant is being filed")
	_ = warrantFileCmd.MarkFlagRequired("reason")
	warrantFileCmd.Flags().StringVar(&warrantFileRequester, "requester", "", "Who is filing the warrant (defaults to BD_ACTOR)")
	warrantFileCmd.Flags().StringVar(&warrantFileSession, "session", "", "Explicit tmux session name for the target")
	warrantFileCmd.Flags().BoolVar(&warrantFileDryRun, "dry-run", false, "Preview without creating a bead")
	warrantFileCmd.Flags().BoolVar(&warrantFileJSON, "json", false, "Output JSON")
}

func runWarrantFile(cmd *cobra.Command, args []string) error {
	target := strings.TrimSpace(args[0])
	if target == "" {
		return fmt.Errorf("target is required")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	requester := resolveWarrantRequester(warrantFileRequester)
	sessionName := warrantFileSession
	if sessionName == "" {
		sessionName = resolveWarrantSession(target)
	}

	filedAt := time.Now().UTC().Format(time.RFC3339)
	title := fmt.Sprintf("Death warrant: %s", target)
	description := formatWarrantDescription(title, target, sessionName, warrantFileReason, requester, filedAt)

	payload := warrantPayload{
		Title:     title,
		Target:    target,
		Session:   sessionName,
		Reason:    warrantFileReason,
		Requester: requester,
		FiledAt:   filedAt,
		DryRun:    warrantFileDryRun,
	}

	if warrantFileDryRun {
		if warrantFileJSON {
			return json.NewEncoder(os.Stdout).Encode(payload)
		}
		fmt.Printf("%s Would file death warrant\n", style.Bold.Render("⚖️"))
		fmt.Printf("  Target: %s\n", target)
		if sessionName != "" {
			fmt.Printf("  Session: %s\n", sessionName)
		}
		fmt.Printf("  Reason: %s\n", warrantFileReason)
		fmt.Printf("  Requester: %s\n", requester)
		return nil
	}

	b := beads.New(townRoot)
	issue, err := b.Create(beads.CreateOptions{
		Title:       title,
		Type:        "warrant",
		Priority:    2,
		Description: description,
		Actor:       requester,
	})
	if err != nil {
		return fmt.Errorf("creating warrant bead: %w", err)
	}

	payload.ID = issue.ID

	if warrantFileJSON {
		return json.NewEncoder(os.Stdout).Encode(payload)
	}

	fmt.Printf("%s Death warrant filed: %s\n", style.Bold.Render("✓"), issue.ID)
	fmt.Printf("  Target: %s\n", target)
	if sessionName != "" {
		fmt.Printf("  Session: %s\n", sessionName)
	}
	fmt.Printf("  Reason: %s\n", warrantFileReason)
	fmt.Printf("  Requester: %s\n", requester)
	return nil
}

func resolveWarrantRequester(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if actor := os.Getenv("BD_ACTOR"); actor != "" {
		return actor
	}
	actor := detectActor()
	if actor != "" && actor != "unknown" {
		return actor
	}
	return "unknown"
}

func resolveWarrantSession(target string) string {
	if strings.HasPrefix(target, "gt-") || strings.HasPrefix(target, "hq-") {
		return target
	}

	switch target {
	case "mayor", "mayor/":
		return session.MayorSessionName()
	case "deacon", "deacon/":
		return session.DeaconSessionName()
	}

	parts := strings.Split(target, "/")
	switch len(parts) {
	case 2:
		rig, role := parts[0], parts[1]
		switch role {
		case "witness":
			return session.WitnessSessionName(rig)
		case "refinery":
			return session.RefinerySessionName(rig)
		}
	case 3:
		rig, role, name := parts[0], parts[1], parts[2]
		switch role {
		case "crew":
			return session.CrewSessionName(rig, name)
		case "polecats":
			return session.PolecatSessionName(rig, name)
		}
	}

	return ""
}

func formatWarrantDescription(title, target, sessionName, reason, requester, filedAt string) string {
	lines := []string{
		title,
		"",
		fmt.Sprintf("target: %s", target),
	}
	if sessionName != "" {
		lines = append(lines, fmt.Sprintf("session: %s", sessionName))
	}
	lines = append(lines,
		fmt.Sprintf("reason: %s", reason),
		fmt.Sprintf("requester: %s", requester),
		fmt.Sprintf("filed_at: %s", filedAt),
	)
	return strings.Join(lines, "\n")
}
