package cmd

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

func townBeadsForRoot(workDir, townRoot string) *beads.Beads {
	if townRoot == "" {
		return nil
	}

	townBeadsDir := filepath.Join(townRoot, ".beads")
	if _, err := os.Stat(townBeadsDir); err != nil {
		return nil
	}

	return beads.NewWithBeadsDir(workDir, townBeadsDir)
}

func townBeadsForWorkDir(workDir string) *beads.Beads {
	townRoot, err := workspace.Find(workDir)
	if err != nil {
		return nil
	}

	return townBeadsForRoot(workDir, townRoot)
}

func showBeadWithFallback(primary *beads.Beads, fallback *beads.Beads, beadID string) (*beads.Issue, *beads.Beads) {
	if primary == nil || beadID == "" {
		return nil, nil
	}

	issue, err := primary.Show(beadID)
	if err == nil && issue != nil {
		return issue, primary
	}

	if fallback == nil {
		return nil, nil
	}

	if err != nil && !errors.Is(err, beads.ErrNotFound) {
		return nil, nil
	}

	issue, err = fallback.Show(beadID)
	if err == nil && issue != nil {
		return issue, fallback
	}

	return nil, nil
}

func listBeadsForAssigneeWithFallback(primary *beads.Beads, fallback *beads.Beads, opts beads.ListOptions) ([]*beads.Issue, *beads.Beads, error) {
	if primary == nil {
		return nil, nil, nil
	}

	issues, err := listBeadsForAssignee(primary, opts)
	if err != nil {
		return nil, nil, err
	}
	if len(issues) > 0 || fallback == nil {
		if len(issues) == 0 {
			return nil, nil, nil
		}
		return issues, primary, nil
	}

	issues, err = listBeadsForAssignee(fallback, opts)
	if err != nil {
		return nil, nil, err
	}
	if len(issues) == 0 {
		return nil, nil, nil
	}

	return issues, fallback, nil
}
