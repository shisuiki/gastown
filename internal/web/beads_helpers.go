package web

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

func webWorkDir() string {
	workDir, err := os.Getwd()
	if err == nil && workDir != "" {
		return workDir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

func webTownRoot() string {
	townRoot, err := workspace.FindFromCwdOrError()
	if err == nil && townRoot != "" {
		return townRoot
	}
	return webWorkDir()
}

func webBeadsEnv(workDir string) []string {
	beadsDir := beads.ResolveBeadsDir(workDir)
	env := append([]string{}, os.Environ()...)
	return append(env, "BEADS_DIR="+beadsDir)
}

func webBeadsArgs(args ...string) []string {
	return append([]string{"--no-daemon", "--allow-stale"}, args...)
}

func normalizeIssueType(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "":
		return "task"
	case "doc":
		return "chore"
	case "bug", "feature", "task", "epic", "chore", "merge-request", "molecule", "gate", "agent", "role", "rig", "convoy", "event":
		return normalized
	default:
		return "task"
	}
}

func generateShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])
}

func parseCreateOutput(out []byte) (string, string) {
	var payload struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &payload); err == nil && payload.ID != "" {
		if payload.Title != "" {
			return fmt.Sprintf("Created %s: %s", payload.ID, payload.Title), payload.ID
		}
		return fmt.Sprintf("Created %s", payload.ID), payload.ID
	}
	return strings.TrimSpace(string(out)), ""
}
