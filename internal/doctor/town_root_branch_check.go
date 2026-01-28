package doctor

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// TownRootBranchCheck verifies that the town root directory is on the configured default branch.
// The town root should always stay on the default branch to avoid confusion and broken gt commands.
// The default branch is configurable via mayor/config.json (default_branch field, defaults to "main").
// Accidental branch switches can happen when git commands run in the wrong directory.
type TownRootBranchCheck struct {
	FixableCheck
	currentBranch string // Cached during Run for use in Fix
	defaultBranch string // Configured default branch for the town
}

// NewTownRootBranchCheck creates a new town root branch check.
func NewTownRootBranchCheck() *TownRootBranchCheck {
	return &TownRootBranchCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "town-root-branch",
				CheckDescription: "Verify town root is on configured default branch",
				CheckCategory:    CategoryCore,
			},
		},
	}
}

// Run checks if the town root is on the configured default branch.
func (c *TownRootBranchCheck) Run(ctx *CheckContext) *CheckResult {
	// Get configured default branch
	c.defaultBranch = config.GetTownDefaultBranch(ctx.TownRoot)

	// Get current branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = ctx.TownRoot
	out, err := cmd.Output()
	if err != nil {
		// Not a git repo - skip this check (handled by town-git check)
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Town root is not a git repository (skipped)",
		}
	}

	branch := strings.TrimSpace(string(out))
	c.currentBranch = branch

	// Empty branch means detached HEAD
	if branch == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Town root is in detached HEAD state",
			Details: []string{
				fmt.Sprintf("The town root should be on the %s branch", c.defaultBranch),
				"Detached HEAD can cause gt commands to fail",
			},
			FixHint: fmt.Sprintf("Run 'gt doctor --fix' or manually: cd ~/gt && git checkout %s", c.defaultBranch),
		}
	}

	// Check if on the configured default branch (or legacy master if default is main)
	if config.IsTownDefaultBranch(ctx.TownRoot, branch) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("Town root is on %s branch", branch),
		}
	}

	// On wrong branch - this is the problem we're trying to prevent
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Town root is on wrong branch: %s", branch),
		Details: []string{
			fmt.Sprintf("The town root (~/gt) must stay on %s branch", c.defaultBranch),
			fmt.Sprintf("Currently on: %s", branch),
			"This can cause gt commands to fail (missing rigs.json, etc.)",
			"The branch switch was likely accidental (git command in wrong dir)",
		},
		FixHint: fmt.Sprintf("Run 'gt doctor --fix' or manually: cd ~/gt && git checkout %s", c.defaultBranch),
	}
}

// Fix switches the town root back to the configured default branch.
func (c *TownRootBranchCheck) Fix(ctx *CheckContext) error {
	// Only fix if we're not already on the default branch
	if config.IsTownDefaultBranch(ctx.TownRoot, c.currentBranch) {
		return nil
	}

	// Check for uncommitted changes that would block checkout
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = ctx.TownRoot
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("cannot switch to %s: uncommitted changes in town root (stash or commit first)", c.defaultBranch)
	}

	// Switch to configured default branch
	cmd = exec.Command("git", "checkout", c.defaultBranch)
	cmd.Dir = ctx.TownRoot
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", c.defaultBranch, err)
	}

	return nil
}
