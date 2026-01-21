package web

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type gitSyncResult struct {
	Error       string `json:"git_error,omitempty"`
	BeadID      string `json:"git_bead,omitempty"`
	SlingTarget string `json:"git_target,omitempty"`
}

func runGitSync(repoRoot string, paths []string, commitMsg string, action string) *gitSyncResult {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return nil
	}

	normalized := normalizeGitPaths(repoRoot, paths)
	if hook := strings.TrimSpace(os.Getenv("GT_WEB_POST_SAVE_HOOK")); hook != "" {
		if err := runGitHook(repoRoot, hook, normalized, commitMsg, action); err != nil {
			return handleGitSyncFailure(repoRoot, action, err)
		}
		return nil
	}

	if err := gitAddCommitPush(repoRoot, normalized, commitMsg); err != nil {
		return handleGitSyncFailure(repoRoot, action, err)
	}

	return nil
}

func normalizeGitPaths(repoRoot string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	repoRoot = filepath.Clean(repoRoot)
	seen := make(map[string]bool)
	var normalized []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		path = filepath.Clean(path)
		if path == repoRoot || !strings.HasPrefix(path, repoRoot+string(os.PathSeparator)) {
			continue
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" {
			continue
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		normalized = append(normalized, rel)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func gitAddCommitPush(repoRoot string, paths []string, commitMsg string) error {
	addArgs := []string{"add"}
	if len(paths) > 0 {
		addArgs = append(addArgs, "--")
		addArgs = append(addArgs, paths...)
	} else {
		addArgs = append(addArgs, "-A")
	}

	cmd, cancel := command("git", addArgs...)
	defer cancel()
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %v, output: %s", err, output)
	}

	if strings.TrimSpace(commitMsg) == "" {
		commitMsg = "Update settings via WebUI"
	}
	cmd, cancel = command("git", "commit", "-m", commitMsg)
	defer cancel()
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %v, output: %s", err, output)
	}

	cmd, cancel = longCommand("git", "push", "origin", "HEAD")
	defer cancel()
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %v, output: %s", err, output)
	}

	return nil
}

func runGitHook(repoRoot, hook string, paths []string, commitMsg, action string) error {
	cmd, cancel := longCommand(hook)
	defer cancel()
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"GT_WEB_GIT_REPO_ROOT="+repoRoot,
		"GT_WEB_GIT_COMMIT_MSG="+commitMsg,
		"GT_WEB_GIT_ACTION="+action,
		"GT_WEB_GIT_PATHS="+strings.Join(paths, ","),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git hook failed: %v, output: %s", err, output)
	}
	return nil
}

func handleGitSyncFailure(repoRoot, action string, err error) *gitSyncResult {
	log.Printf("Git sync failed for %s: %v", action, err)
	title := fmt.Sprintf("WebUI git sync failed: %s", action)
	desc := fmt.Sprintf("Action: %s\nRepo: %s\nError: %v\nTime: %s\n",
		action,
		repoRoot,
		err,
		time.Now().UTC().Format(time.RFC3339),
	)
	beadID, beadErr := createGitFailureBead(repoRoot, title, desc)
	if beadErr != nil {
		log.Printf("Failed to create bead for git sync failure: %v", beadErr)
		return &gitSyncResult{Error: err.Error()}
	}

	target := resolveGitFailureTarget()
	if target != "" {
		if slingErr := slingGitFailure(beadID, target, action, err); slingErr != nil {
			log.Printf("Failed to sling git sync failure bead: %v", slingErr)
		}
	}

	return &gitSyncResult{
		Error:       err.Error(),
		BeadID:      beadID,
		SlingTarget: target,
	}
}

func createGitFailureBead(repoRoot, title, description string) (string, error) {
	cmd, cancel := command("bd", "new",
		"--type", "bug",
		"--priority", "1",
		"--title", title,
		"--description", description,
		"--labels", "webui,git",
		"--silent",
	)
	defer cancel()
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bd new failed: %v, output: %s", err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

func resolveGitFailureTarget() string {
	if target := strings.TrimSpace(os.Getenv("GT_WEB_GIT_FAILOVER_TARGET")); target != "" {
		return target
	}
	return strings.TrimSpace(os.Getenv("GT_RIG"))
}

func slingGitFailure(beadID, target, action string, err error) error {
	msg := fmt.Sprintf("Git sync failed for %s: %v", action, err)
	cmd, cancel := command("gt", "sling", beadID, target, "--message", msg)
	defer cancel()
	output, slingErr := cmd.CombinedOutput()
	if slingErr != nil {
		return fmt.Errorf("gt sling failed: %v, output: %s", slingErr, output)
	}
	return nil
}
