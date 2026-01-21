package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var deaconZombieScanCmd = &cobra.Command{
	Use:   "zombie-scan",
	Short: "Detect zombie polecats (read-only)",
	Long: `Detect zombie polecats that appear abandoned and ready for warrant filing.

This command is read-only. It never kills sessions or files warrants. It reports
polecats with:
- No active tmux session
- No assigned or hooked work
- No uncommitted work
- Last bead update older than the idle threshold (default 10m)

Example:
  gt deacon zombie-scan --dry-run`,
	RunE: runDeaconZombieScan,
}

var (
	zombieScanDryRun bool
	zombieScanJSON   bool
	zombieScanIdle   time.Duration
	zombieScanRig    string
)

type zombieScanEntry struct {
	Rig                string `json:"rig"`
	Polecat            string `json:"polecat"`
	Session            string `json:"session"`
	HasSession         bool   `json:"has_session"`
	HasHook            bool   `json:"has_hook"`
	HasAssignedIssue   bool   `json:"has_assigned_issue"`
	HasUncommittedWork bool   `json:"has_uncommitted_work"`
	AgentState         string `json:"agent_state,omitempty"`
	LastUpdate         string `json:"last_update,omitempty"`
	IdleFor            string `json:"idle_for,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type zombieScanResult struct {
	ScannedAt     time.Time         `json:"scanned_at"`
	IdleThreshold string            `json:"idle_threshold"`
	TotalPolecats int               `json:"total_polecats"`
	ZombieCount   int               `json:"zombie_count"`
	Zombies       []zombieScanEntry `json:"zombies"`
}

func init() {
	deaconCmd.AddCommand(deaconZombieScanCmd)

	deaconZombieScanCmd.Flags().BoolVar(&zombieScanDryRun, "dry-run", false, "Preview only (required)")
	deaconZombieScanCmd.Flags().BoolVar(&zombieScanJSON, "json", false, "Output JSON")
	deaconZombieScanCmd.Flags().DurationVar(&zombieScanIdle, "idle", 10*time.Minute, "Idle threshold for last activity")
	deaconZombieScanCmd.Flags().StringVar(&zombieScanRig, "rig", "", "Scan a single rig by name")
}

func runDeaconZombieScan(cmd *cobra.Command, args []string) error {
	if !zombieScanDryRun {
		return fmt.Errorf("zombie-scan is read-only; use --dry-run")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigs, err := workspace.ListRigs(townRoot)
	if err != nil {
		return fmt.Errorf("listing rigs: %w", err)
	}
	if zombieScanRig != "" {
		rigs = filterRigList(rigs, zombieScanRig)
		if len(rigs) == 0 {
			return fmt.Errorf("rig %q not found", zombieScanRig)
		}
	}

	result := zombieScanResult{
		ScannedAt:     time.Now().UTC(),
		IdleThreshold: zombieScanIdle.String(),
	}

	t := tmux.NewTmux()

	for _, rigInfo := range rigs {
		r := &rig.Rig{Name: rigInfo.Name, Path: rigInfo.Path}
		g := git.NewGit(r.Path)
		mgr := polecat.NewManager(r, g, t)

		polecats, err := mgr.List()
		if err != nil {
			fmt.Printf("%s %s: %v\n", style.Warning.Render("⚠"), rigInfo.Name, err)
			continue
		}
		if len(polecats) == 0 {
			continue
		}

		infoByName := make(map[string]*polecat.StalenessInfo, len(polecats))
		staleInfos, err := mgr.DetectStalePolecats(20)
		if err == nil {
			for _, info := range staleInfos {
				infoByName[info.Name] = info
			}
		}

		b := beads.New(r.Path)
		prefix := beads.GetPrefixForRig(townRoot, r.Name)

		for _, p := range polecats {
			result.TotalPolecats++
			entry := zombieScanEntry{
				Rig:     r.Name,
				Polecat: p.Name,
				Session: session.PolecatSessionName(r.Name, p.Name),
			}

			if info := infoByName[p.Name]; info != nil {
				entry.HasSession = info.HasActiveSession
				entry.HasUncommittedWork = info.HasUncommittedWork
				entry.AgentState = info.AgentState
			}

			if entry.HasSession || entry.HasUncommittedWork {
				continue
			}
			if entry.AgentState == "stuck" || entry.AgentState == "awaiting-gate" {
				continue
			}

			assignee := fmt.Sprintf("%s/polecats/%s", r.Name, p.Name)
			assignedIssue, _ := b.GetAssignedIssue(assignee)
			entry.HasAssignedIssue = assignedIssue != nil
			hookedIssues, _ := listBeadsForAssignee(b, beads.ListOptions{
				Status:   beads.StatusHooked,
				Assignee: assignee,
				Priority: -1,
			})
			entry.HasHook = len(hookedIssues) > 0

			if entry.HasAssignedIssue || entry.HasHook {
				continue
			}

			beadID := beads.PolecatBeadIDWithPrefix(prefix, r.Name, p.Name)
			lastUpdate := time.Time{}
			if issue, showErr := b.Show(beadID); showErr == nil && issue != nil {
				lastUpdate = parseBeadsTimestampLoose(issue.UpdatedAt)
			}

			idleFor := time.Duration(0)
			if lastUpdate.IsZero() {
				idleFor = zombieScanIdle + time.Second
			} else {
				idleFor = time.Since(lastUpdate)
				entry.LastUpdate = lastUpdate.Format(time.RFC3339)
				entry.IdleFor = idleFor.Round(time.Minute).String()
			}

			if idleFor < zombieScanIdle {
				continue
			}

			entry.Reason = "no session, no hook, no assigned issue"
			if lastUpdate.IsZero() {
				entry.Reason = "no session, no hook, no assigned issue, no agent bead update"
			}
			if entry.IdleFor == "" {
				entry.IdleFor = idleFor.Round(time.Minute).String()
			}

			result.Zombies = append(result.Zombies, entry)
		}
	}

	result.ZombieCount = len(result.Zombies)

	if zombieScanJSON {
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	fmt.Printf("Detecting zombie polecats (idle > %s)...\n\n", zombieScanIdle)
	if result.ZombieCount == 0 {
		fmt.Printf("%s No zombies detected\n", style.SuccessPrefix)
		return nil
	}

	for _, entry := range result.Zombies {
		fmt.Printf("%s %s/%s\n", style.Warning.Render("○"), entry.Rig, entry.Polecat)
		fmt.Printf("    Session: %s\n", style.Dim.Render("stopped"))
		if entry.IdleFor != "" {
			fmt.Printf("    Idle: %s\n", entry.IdleFor)
		}
		if entry.LastUpdate != "" {
			fmt.Printf("    Last update: %s\n", entry.LastUpdate)
		}
		fmt.Printf("    Reason: %s\n\n", entry.Reason)
	}

	fmt.Printf("Summary: %d zombie(s) out of %d polecats\n", result.ZombieCount, result.TotalPolecats)
	return nil
}

func filterRigList(rigs []workspace.RigInfo, rigName string) []workspace.RigInfo {
	var filtered []workspace.RigInfo
	for _, rigInfo := range rigs {
		if rigInfo.Name == rigName {
			filtered = append(filtered, rigInfo)
		}
	}
	return filtered
}

func parseBeadsTimestampLoose(s string) time.Time {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
