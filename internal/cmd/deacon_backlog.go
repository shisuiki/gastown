package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
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

var deaconBacklogDispatchCmd = &cobra.Command{
	Use:   "backlog-dispatch",
	Short: "Dispatch ready work to idle polecats",
	Long: `Dispatch ready work items to idle polecats.

Pairs ready (unblocked, unassigned) issues with idle polecats that have
no active work, no hook, and no uncommitted changes.

This command never kills sessions.`,
	RunE: runDeaconBacklogDispatch,
}

var (
	backlogDispatchDryRun  bool
	backlogDispatchJSON    bool
	backlogDispatchNotify  bool
	backlogDispatchRig     string
	backlogDispatchLimit   int
	backlogDispatchVerbose bool
)

type idlePolecat struct {
	Rig        string `json:"rig"`
	Name       string `json:"name"`
	Session    string `json:"session"`
	ClonePath  string `json:"clone_path"`
	Attached   bool   `json:"attached"`
	SkipReason string `json:"skip_reason,omitempty"`
}

type readyIssue struct {
	Issue     *beads.Issue `json:"issue"`
	TargetRig string       `json:"target_rig,omitempty"`
}

type dispatchRecord struct {
	IssueID string `json:"issue_id"`
	Target  string `json:"target"`
	Error   string `json:"error,omitempty"`
}

type backlogDispatchResult struct {
	ScannedAt       time.Time        `json:"scanned_at"`
	IdlePolecats    int              `json:"idle_polecats"`
	ReadyIssues     int              `json:"ready_issues"`
	Dispatched      int              `json:"dispatched"`
	SkippedIssues   int              `json:"skipped_issues"`
	SkippedPolecats int              `json:"skipped_polecats"`
	Records         []dispatchRecord `json:"records,omitempty"`
}

func init() {
	deaconCmd.AddCommand(deaconBacklogDispatchCmd)

	deaconBacklogDispatchCmd.Flags().BoolVar(&backlogDispatchDryRun, "dry-run", false, "Preview without slinging work")
	deaconBacklogDispatchCmd.Flags().BoolVar(&backlogDispatchJSON, "json", false, "Output JSON")
	deaconBacklogDispatchCmd.Flags().BoolVar(&backlogDispatchNotify, "notify", true, "Notify mayor when backlog is exhausted or idle capacity is missing")
	deaconBacklogDispatchCmd.Flags().StringVar(&backlogDispatchRig, "rig", "", "Restrict dispatch to a single rig")
	deaconBacklogDispatchCmd.Flags().IntVar(&backlogDispatchLimit, "limit", 0, "Maximum number of dispatches (0 = no limit)")
	deaconBacklogDispatchCmd.Flags().BoolVar(&backlogDispatchVerbose, "verbose", false, "Show skipped polecats and issues")
}

func runDeaconBacklogDispatch(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigs, err := workspace.ListRigs(townRoot)
	if err != nil {
		return fmt.Errorf("listing rigs: %w", err)
	}
	if backlogDispatchRig != "" {
		rigs = filterRigList(rigs, backlogDispatchRig)
		if len(rigs) == 0 {
			return fmt.Errorf("rig %q not found", backlogDispatchRig)
		}
	}

	t := tmux.NewTmux()
	idleByRig := make(map[string][]idlePolecat)
	var idleAll []idlePolecat
	var skippedPolecats int

	for _, rigInfo := range rigs {
		r := &rig.Rig{Name: rigInfo.Name, Path: rigInfo.Path}
		g := git.NewGit(r.Path)
		mgr := polecat.NewManager(r, g, t)

		polecats, err := mgr.List()
		if err != nil {
			fmt.Printf("%s %s: %v\n", style.Warning.Render("⚠"), rigInfo.Name, err)
			continue
		}

		b := beads.New(r.Path)

		for _, p := range polecats {
			entry := idlePolecat{
				Rig:       r.Name,
				Name:      p.Name,
				Session:   session.PolecatSessionName(r.Name, p.Name),
				ClonePath: p.ClonePath,
			}

			if p.State != polecat.StateDone {
				entry.SkipReason = "active"
				skippedPolecats++
				maybeReportSkip(entry)
				continue
			}

			sessionInfo, sessErr := t.GetSessionInfo(entry.Session)
			if sessErr != nil {
				entry.SkipReason = "no session"
				skippedPolecats++
				maybeReportSkip(entry)
				continue
			}
			entry.Attached = sessionInfo.Attached
			if sessionInfo.Attached {
				entry.SkipReason = "attached"
				skippedPolecats++
				maybeReportSkip(entry)
				continue
			}

			workStatus, workErr := git.NewGit(p.ClonePath).CheckUncommittedWork()
			if workErr == nil && !workStatus.CleanExcludingBeads() {
				entry.SkipReason = "uncommitted work"
				skippedPolecats++
				maybeReportSkip(entry)
				continue
			}

			assignee := fmt.Sprintf("%s/polecats/%s", r.Name, p.Name)
			if issue, _ := b.GetAssignedIssue(assignee); issue != nil {
				entry.SkipReason = "assigned issue"
				skippedPolecats++
				maybeReportSkip(entry)
				continue
			}

			hooked, _ := listBeadsForAssignee(b, beads.ListOptions{
				Status:   beads.StatusHooked,
				Assignee: assignee,
				Priority: -1,
			})
			if len(hooked) > 0 {
				entry.SkipReason = "hooked"
				skippedPolecats++
				maybeReportSkip(entry)
				continue
			}

			idleByRig[r.Name] = append(idleByRig[r.Name], entry)
			idleAll = append(idleAll, entry)
		}
	}

	readyIssues := collectReadyIssues(townRoot, rigs)
	if backlogDispatchRig != "" {
		readyIssues = filterReadyIssuesForRig(readyIssues, backlogDispatchRig)
	}

	sortReadyIssues(readyIssues)

	result := backlogDispatchResult{
		ScannedAt:       time.Now().UTC(),
		IdlePolecats:    len(idleAll),
		ReadyIssues:     len(readyIssues),
		SkippedPolecats: skippedPolecats,
	}

	if len(idleAll) == 0 || len(readyIssues) == 0 {
		if backlogDispatchJSON {
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		printBacklogSummary(result)
		if backlogDispatchNotify && !backlogDispatchDryRun {
			notifyBacklog(townRoot, len(idleAll), len(readyIssues), 0, 0)
		}
		return nil
	}

	dispatchLimit := backlogDispatchLimit
	if dispatchLimit <= 0 {
		dispatchLimit = len(readyIssues)
	}

	for _, item := range readyIssues {
		if result.Dispatched >= dispatchLimit {
			result.SkippedIssues += len(readyIssues) - result.Dispatched
			break
		}

		targetRig := item.TargetRig
		var chosen idlePolecat
		var ok bool

		if targetRig != "" {
			chosen, ok = popIdle(&idleByRig, &idleAll, targetRig)
		} else {
			chosen, ok = popIdleAny(&idleByRig, &idleAll)
		}

		if !ok {
			result.SkippedIssues++
			continue
		}

		target := fmt.Sprintf("%s/polecats/%s", chosen.Rig, chosen.Name)
		record := dispatchRecord{IssueID: item.Issue.ID, Target: target}

		if backlogDispatchDryRun {
			result.Dispatched++
			result.Records = append(result.Records, record)
			continue
		}

		if err := slingIssue(townRoot, item.Issue.ID, target); err != nil {
			record.Error = err.Error()
		} else {
			result.Dispatched++
		}
		result.Records = append(result.Records, record)
	}

	if backlogDispatchJSON {
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	printBacklogSummary(result)
	for _, record := range result.Records {
		if record.Error != "" {
			fmt.Printf("  %s %s → %s (%s)\n", style.Warning.Render("⚠"), record.IssueID, record.Target, record.Error)
			continue
		}
		if backlogDispatchDryRun {
			fmt.Printf("  %s %s → %s\n", style.Dim.Render("○"), record.IssueID, record.Target)
		} else {
			fmt.Printf("  %s %s → %s\n", style.Success.Render("✓"), record.IssueID, record.Target)
		}
	}

	if backlogDispatchNotify && !backlogDispatchDryRun {
		notifyBacklog(townRoot, len(idleAll)+result.Dispatched, len(readyIssues), result.Dispatched, result.SkippedIssues)
	}

	return nil
}

func maybeReportSkip(entry idlePolecat) {
	if !backlogDispatchVerbose || entry.SkipReason == "" {
		return
	}
	fmt.Printf("  %s %s/%s (%s)\n", style.Dim.Render("○"), entry.Rig, entry.Name, entry.SkipReason)
}

func collectReadyIssues(townRoot string, rigs []workspace.RigInfo) []readyIssue {
	seen := make(map[string]bool)
	var ready []readyIssue

	addIssues := func(source string, issues []*beads.Issue) {
		for _, issue := range issues {
			if issue == nil || issue.ID == "" {
				continue
			}
			if seen[issue.ID] {
				continue
			}
			if issue.Assignee != "" {
				continue
			}
			if issue.Status != "" && issue.Status != "open" && issue.Status != "ready" {
				continue
			}
			targetRig := resolveIssueRig(townRoot, source, issue.ID, rigs)
			ready = append(ready, readyIssue{Issue: issue, TargetRig: targetRig})
			seen[issue.ID] = true
		}
	}

	townBeads := beads.New(townRoot)
	if issues, err := townBeads.Ready(); err == nil {
		addIssues("town", issues)
	}

	for _, rigInfo := range rigs {
		rigBeads := beads.New(rigInfo.Path)
		if issues, err := rigBeads.Ready(); err == nil {
			addIssues(rigInfo.Name, issues)
		}
	}

	return ready
}

func resolveIssueRig(townRoot, source, issueID string, rigs []workspace.RigInfo) string {
	prefix := beads.ExtractPrefix(issueID)
	if prefix == "" {
		return source
	}

	rigPath := beads.GetRigPathForPrefix(townRoot, prefix)
	if rigPath == "" || rigPath == townRoot {
		return ""
	}

	for _, rigInfo := range rigs {
		if rigInfo.Path == rigPath {
			return rigInfo.Name
		}
	}

	return source
}

func filterReadyIssuesForRig(issues []readyIssue, rigName string) []readyIssue {
	var filtered []readyIssue
	for _, item := range issues {
		if item.TargetRig == "" || item.TargetRig == rigName {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func sortReadyIssues(issues []readyIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		pi := issues[i].Issue.Priority
		pj := issues[j].Issue.Priority
		if pi != pj {
			return pi < pj
		}
		ti := parseBeadsTimestamp(issues[i].Issue.CreatedAt)
		tj := parseBeadsTimestamp(issues[j].Issue.CreatedAt)
		if !ti.IsZero() && !tj.IsZero() {
			return ti.Before(tj)
		}
		return issues[i].Issue.ID < issues[j].Issue.ID
	})
}

func popIdle(idleByRig *map[string][]idlePolecat, idleAll *[]idlePolecat, rigName string) (idlePolecat, bool) {
	pool := (*idleByRig)[rigName]
	if len(pool) == 0 {
		return idlePolecat{}, false
	}
	chosen := pool[0]
	(*idleByRig)[rigName] = pool[1:]
	removeIdleGlobal(idleAll, chosen)
	return chosen, true
}

func popIdleAny(idleByRig *map[string][]idlePolecat, idleAll *[]idlePolecat) (idlePolecat, bool) {
	if len(*idleAll) == 0 {
		return idlePolecat{}, false
	}
	chosen := (*idleAll)[0]
	*idleAll = (*idleAll)[1:]
	rigPool := (*idleByRig)[chosen.Rig]
	for i := range rigPool {
		if rigPool[i].Name == chosen.Name {
			(*idleByRig)[chosen.Rig] = append(rigPool[:i], rigPool[i+1:]...)
			break
		}
	}
	return chosen, true
}

func removeIdleGlobal(idleAll *[]idlePolecat, chosen idlePolecat) {
	for i, entry := range *idleAll {
		if entry.Rig == chosen.Rig && entry.Name == chosen.Name {
			*idleAll = append((*idleAll)[:i], (*idleAll)[i+1:]...)
			return
		}
	}
}

func slingIssue(townRoot, issueID, target string) error {
	cmd := exec.Command("gt", "sling", issueID, target)
	cmd.Dir = townRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printBacklogSummary(result backlogDispatchResult) {
	fmt.Printf("Backlog dispatch: %d ready, %d idle, %d dispatched\n",
		result.ReadyIssues, result.IdlePolecats, result.Dispatched)
	if result.SkippedIssues > 0 {
		fmt.Printf("%s %d issue(s) left undispatched\n", style.Dim.Render("○"), result.SkippedIssues)
	}
}

func notifyBacklog(townRoot string, idleCount, readyCount, dispatched, skipped int) {
	switch {
	case readyCount == 0 && idleCount > 0:
		sendMail(townRoot, "mayor/", "Backlog dispatch: backlog exhausted",
			fmt.Sprintf("No ready issues found. Idle polecats: %d.", idleCount))
	case readyCount > 0 && idleCount == 0 && dispatched == 0:
		sendMail(townRoot, "mayor/", "Backlog dispatch: no idle polecats",
			fmt.Sprintf("Ready issues: %d. No idle polecats available.", readyCount))
	case skipped > 0:
		sendMail(townRoot, "mayor/", "Backlog dispatch: insufficient idle capacity",
			fmt.Sprintf("Ready issues: %d. Dispatched: %d. Remaining: %d.", readyCount, dispatched, skipped))
	}
}
