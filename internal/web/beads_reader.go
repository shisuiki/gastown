package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

// BeadsReader provides access to beads via bd CLI commands.
type BeadsReader struct {
	townRoot string
	workDir  string
	beadsDir string
}

// Bead represents a bead from the database.
type Bead struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Description  string           `json:"description,omitempty"`
	Status       string           `json:"status"`
	Priority     int              `json:"priority"`
	Type         string           `json:"issue_type"`
	Owner        string           `json:"owner,omitempty"`
	Assignee     string           `json:"assignee,omitempty"`
	Labels       []string         `json:"labels,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	ClosedAt     *time.Time       `json:"closed_at,omitempty"`
	Ephemeral    bool             `json:"ephemeral,omitempty"`
	Dependencies []BeadDependency `json:"dependencies,omitempty"`
}

// BeadDependency represents a dependency between beads.
type BeadDependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"` // "blocks", "tracks", "parent-child"
}

// AgentHook represents an agent's hook status.
type AgentHook struct {
	Agent     string `json:"agent"` // e.g., "TerraNomadicCity/crew/Myrtle"
	Role      string `json:"role"`  // e.g., "crew"
	HasWork   bool   `json:"has_work"`
	WorkType  string `json:"work_type"` // "molecule", "mail", "none"
	WorkID    string `json:"work_id,omitempty"`
	WorkTitle string `json:"work_title,omitempty"`
}

// NewBeadsReader creates a BeadsReader for the given town root.
func NewBeadsReader(townRoot string) (*BeadsReader, error) {
	return newBeadsReader(townRoot, "")
}

// NewBeadsReaderWithBeadsDir creates a BeadsReader that targets a specific beads directory.
func NewBeadsReaderWithBeadsDir(townRoot, beadsDir string) (*BeadsReader, error) {
	return newBeadsReader(townRoot, beadsDir)
}

func newBeadsReader(townRoot, beadsDir string) (*BeadsReader, error) {
	if townRoot == "" {
		townRoot = os.Getenv("GT_ROOT")
		if townRoot == "" {
			home, _ := os.UserHomeDir()
			townRoot = filepath.Join(home, "gt")
		}
	}

	workDir, err := os.Getwd()
	if err != nil {
		workDir = townRoot
	}

	if beadsDir == "" {
		beadsDir = beads.ResolveBeadsDir(workDir)
	}

	return &BeadsReader{
		townRoot: townRoot,
		workDir:  workDir,
		beadsDir: beadsDir,
	}, nil
}

// BeadFilter specifies criteria for listing beads.
type BeadFilter struct {
	Status           string
	Type             string
	Assignee         string
	ExcludeTypes     []string
	IncludeEphemeral bool
	Priority         int
	Limit            int
}

// ListBeads returns beads matching the given filters using bd list --json.
func (r *BeadsReader) ListBeads(filter BeadFilter) ([]Bead, error) {
	args := []string{"list", "--json"}

	if filter.Status != "" {
		args = append(args, "--status="+filter.Status)
	}
	if filter.Type != "" {
		args = append(args, "--type="+filter.Type)
	}
	if filter.Assignee != "" {
		args = append(args, "--assignee="+filter.Assignee)
	}
	if filter.Limit > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", filter.Limit))
	} else {
		args = append(args, "--limit=100")
	}

	var beads []Bead
	if jsonl, err := r.readIssuesJSONL(); err == nil {
		beads = jsonl
	} else {
		cmd, cancel := r.beadsCommand(args...)
		defer cancel()
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("bd list failed: %w", err)
		}

		if err := json.Unmarshal(output, &beads); err != nil {
			return nil, fmt.Errorf("parse error: %w", err)
		}
	}

	// Filter out excluded types and ephemeral if needed
	filtered := make([]Bead, 0, len(beads))
	for _, b := range beads {
		if filter.Status != "" && b.Status != filter.Status {
			continue
		}
		if filter.Type != "" && b.Type != filter.Type {
			continue
		}
		if filter.Assignee != "" && b.Assignee != filter.Assignee {
			continue
		}
		// Skip excluded types
		if len(filter.ExcludeTypes) > 0 {
			skip := false
			for _, t := range filter.ExcludeTypes {
				if b.Type == t {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		// Skip ephemeral unless requested
		if b.Ephemeral && !filter.IncludeEphemeral {
			continue
		}
		filtered = append(filtered, b)
		if filter.Limit > 0 && len(filtered) >= filter.Limit {
			break
		}
	}

	return filtered, nil
}

// ReadyBeads returns open beads that are not blocked by dependencies.
func (r *BeadsReader) ReadyBeads() ([]Bead, error) {
	if jsonl, err := r.readIssuesJSONL(); err == nil {
		statusByID := make(map[string]string, len(jsonl))
		for _, b := range jsonl {
			statusByID[b.ID] = b.Status
		}

		ready := make([]Bead, 0, len(jsonl))
		for _, b := range jsonl {
			if b.Status != "open" || b.Ephemeral {
				continue
			}
			blocked := false
			for _, dep := range b.Dependencies {
				depID := dep.DependsOnID
				if depID == "" {
					continue
				}
				if strings.HasPrefix(depID, "external:") {
					blocked = true
					break
				}
				if status, ok := statusByID[depID]; !ok || status != "closed" {
					blocked = true
					break
				}
			}
			if !blocked {
				ready = append(ready, b)
			}
		}

		sort.Slice(ready, func(i, j int) bool {
			pi := ready[i].Priority
			pj := ready[j].Priority
			if pi == 0 {
				pi = 99
			}
			if pj == 0 {
				pj = 99
			}
			if pi != pj {
				return pi < pj
			}
			if !ready[i].CreatedAt.IsZero() && !ready[j].CreatedAt.IsZero() && !ready[i].CreatedAt.Equal(ready[j].CreatedAt) {
				return ready[i].CreatedAt.Before(ready[j].CreatedAt)
			}
			return ready[i].ID < ready[j].ID
		})

		return ready, nil
	}

	cmd, cancel := r.beadsCommand("ready", "--json")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd ready failed: %w", err)
	}

	var beads []Bead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return beads, nil
}

// GetBead returns a single bead by ID using bd show --json.
func (r *BeadsReader) GetBead(id string) (*Bead, error) {
	if jsonl, err := r.readIssuesJSONL(); err == nil {
		for _, b := range jsonl {
			if b.ID == id {
				bead := b
				return &bead, nil
			}
		}
	}

	cmd, cancel := r.beadsCommand("show", id, "--json")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd show failed: %w", err)
	}

	var beads []Bead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	if len(beads) == 0 {
		return nil, fmt.Errorf("bead not found: %s", id)
	}

	return &beads[0], nil
}

// GetConvoyTrackedIssues returns the issues tracked by a convoy.
// Convoys use dependency type "tracks" to link to their issues.
func (r *BeadsReader) GetConvoyTrackedIssues(convoyID string) ([]Bead, error) {
	if jsonl, err := r.readIssuesJSONL(); err == nil {
		beadsByID := make(map[string]Bead, len(jsonl))
		for _, b := range jsonl {
			beadsByID[b.ID] = b
		}

		if convoy, ok := beadsByID[convoyID]; ok {
			tracked := make([]Bead, 0, len(convoy.Dependencies))
			for _, dep := range convoy.Dependencies {
				if dep.Type != "tracks" {
					continue
				}
				issueID := dep.DependsOnID
				if strings.HasPrefix(issueID, "external:") {
					parts := strings.SplitN(issueID, ":", 3)
					if len(parts) == 3 {
						issueID = parts[2]
					}
				}
				if issueID == "" {
					continue
				}
				if bead, found := beadsByID[issueID]; found {
					tracked = append(tracked, bead)
				} else {
					tracked = append(tracked, Bead{
						ID:    issueID,
						Title: "(external)",
					})
				}
			}
			return tracked, nil
		}
	}

	// Use bd show with --json to get the convoy and its dependents
	cmd, cancel := r.beadsCommand("show", convoyID, "--json")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd show failed: %w", err)
	}

	var results []struct {
		ID         string `json:"id"`
		Dependents []struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Status         string `json:"status"`
			Priority       int    `json:"priority"`
			Type           string `json:"issue_type"`
			DependencyType string `json:"dependency_type"`
		} `json:"dependents"`
	}

	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	if len(results) == 0 {
		return []Bead{}, nil
	}

	// Extract tracked issues (dependency_type = "tracks")
	var beads []Bead
	for _, dep := range results[0].Dependents {
		if dep.DependencyType == "tracks" {
			beads = append(beads, Bead{
				ID:       dep.ID,
				Title:    dep.Title,
				Status:   dep.Status,
				Priority: dep.Priority,
				Type:     dep.Type,
			})
		}
	}

	return beads, nil
}

// GetAllAgentHooks returns hook status for all active agents.
func (r *BeadsReader) GetAllAgentHooks() ([]AgentHook, error) {
	// List all tmux sessions to find active agents
	cmd, cancel := command("tmux", "list-sessions", "-F", "#{session_name}")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var hooks []AgentHook
	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, session := range sessions {
		if session == "" {
			continue
		}

		// Parse session name to identify agents
		var agent, role string

		if strings.HasPrefix(session, "hq-") {
			// Mayor, deacon, etc.
			role = strings.TrimPrefix(session, "hq-")
			agent = role + "/"
		} else if strings.HasPrefix(session, "gt-") {
			parts := strings.SplitN(session, "-", 4)
			if len(parts) < 3 {
				continue
			}
			rig := parts[1]

			if parts[2] == "crew" && len(parts) >= 4 {
				role = "crew"
				agent = fmt.Sprintf("%s/crew/%s", rig, parts[3])
			} else if parts[2] == "witness" || parts[2] == "refinery" {
				role = parts[2]
				agent = fmt.Sprintf("%s/%s/", rig, parts[2])
			} else {
				role = "polecat"
				agent = fmt.Sprintf("%s/polecats/%s", rig, parts[2])
			}
		} else {
			continue
		}

		// Get hook status for this agent
		hook := r.getAgentHook(agent, role)
		hooks = append(hooks, hook)
	}

	return hooks, nil
}

// getAgentHook gets the hook status for a specific agent.
func (r *BeadsReader) getAgentHook(agent, role string) AgentHook {
	hook := AgentHook{
		Agent:    agent,
		Role:     role,
		HasWork:  false,
		WorkType: "none",
	}

	type hookStatusJSON struct {
		Target           string `json:"target"`
		Role             string `json:"role"`
		HasWork          bool   `json:"has_work"`
		AttachedMolecule string `json:"attached_molecule,omitempty"`
		PinnedBead       *Bead  `json:"pinned_bead,omitempty"`
	}

	cmd, cancel := command("gt", "hook", "status", agent, "--json")
	defer cancel()
	output, err := cmd.Output()
	if err == nil {
		var parsed hookStatusJSON
		if json.Unmarshal(output, &parsed) == nil {
			if parsed.Target != "" {
				hook.Agent = parsed.Target
			}
			if parsed.Role != "" {
				hook.Role = parsed.Role
			}
			hook.HasWork = parsed.HasWork

			if parsed.PinnedBead != nil {
				hook.WorkID = parsed.PinnedBead.ID
				hook.WorkTitle = parsed.PinnedBead.Title
				if parsed.PinnedBead.Type != "" {
					hook.WorkType = parsed.PinnedBead.Type
				}
			}

			if parsed.AttachedMolecule != "" {
				hook.WorkType = "molecule"
			} else if hook.WorkType == "none" && hook.HasWork {
				hook.WorkType = "hooked"
			}

			return hook
		}
	}

	// Try to get hook info by running gt hook with actor context
	cmd, cancel = command("gt", "hook", "show", agent)
	defer cancel()
	output, err = cmd.Output()
	if err != nil {
		return hook
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Nothing on hook") {
			break
		}

		// Check for molecule
		if strings.Contains(line, "📦") || strings.Contains(line, "Molecule:") {
			hook.HasWork = true
			hook.WorkType = "molecule"
			// Try to extract ID
			if idx := strings.Index(line, ":"); idx != -1 {
				hook.WorkID = strings.TrimSpace(line[idx+1:])
			}
		}

		// Check for mail
		if strings.Contains(line, "📬") || strings.Contains(line, "📧") || strings.Contains(line, "Mail:") {
			hook.HasWork = true
			hook.WorkType = "mail"
			if idx := strings.Index(line, ":"); idx != -1 {
				hook.WorkID = strings.TrimSpace(line[idx+1:])
			}
		}
	}

	return hook
}

// GetBeadDependencies returns all dependencies for a bead.
func (r *BeadsReader) GetBeadDependencies(beadID string) ([]BeadDependency, error) {
	if jsonl, err := r.readIssuesJSONL(); err == nil {
		for _, bead := range jsonl {
			if bead.ID == beadID {
				return bead.Dependencies, nil
			}
		}
	}

	// Use bd show --json to get dependencies
	cmd, cancel := r.beadsCommand("show", beadID, "--json")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd show failed: %w", err)
	}

	var results []struct {
		ID         string `json:"id"`
		Dependents []struct {
			ID             string `json:"id"`
			DependencyType string `json:"dependency_type"`
		} `json:"dependents"`
	}

	if err := json.Unmarshal(output, &results); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return []BeadDependency{}, nil
	}

	var deps []BeadDependency
	for _, dep := range results[0].Dependents {
		deps = append(deps, BeadDependency{
			IssueID:     beadID,
			DependsOnID: dep.ID,
			Type:        dep.DependencyType,
		})
	}

	return deps, nil
}

// GetBeadStats returns statistics about beads.
func (r *BeadsReader) GetBeadStats() (map[string]int, error) {
	stats := make(map[string]int)

	var beads []Bead
	if jsonl, err := r.readIssuesJSONL(); err == nil {
		beads = jsonl
	} else {
		// Get all beads with JSON
		cmd, cancel := r.beadsCommand("list", "--json", "--limit=0")
		defer cancel()
		output, err := cmd.Output()
		if err != nil {
			return stats, err
		}

		if err := json.Unmarshal(output, &beads); err != nil {
			return stats, err
		}
	}

	// Count by status and type
	for _, b := range beads {
		if !b.Ephemeral {
			stats["status_"+b.Status]++
			stats["type_"+b.Type]++
			stats["total"]++
		}
	}

	return stats, nil
}

// SearchBeads searches beads by text in title and description.
func (r *BeadsReader) SearchBeads(searchQuery string, limit int) ([]Bead, error) {
	if jsonl, err := r.readIssuesJSONL(); err == nil {
		needle := strings.ToLower(searchQuery)
		matches := make([]Bead, 0)
		for _, b := range jsonl {
			if needle == "" {
				continue
			}
			if strings.Contains(strings.ToLower(b.ID), needle) ||
				strings.Contains(strings.ToLower(b.Title), needle) ||
				strings.Contains(strings.ToLower(b.Description), needle) {
				matches = append(matches, b)
				if limit > 0 && len(matches) >= limit {
					break
				}
			}
		}
		return matches, nil
	}

	args := []string{"search", searchQuery, "--json"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", limit))
	}

	cmd, cancel := r.beadsCommand(args...)
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		// Search might return empty results
		return []Bead{}, nil
	}

	var beads []Bead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, err
	}

	return beads, nil
}

type issueJSONL struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	Status       string           `json:"status"`
	Priority     int              `json:"priority"`
	Type         string           `json:"issue_type"`
	Owner        string           `json:"owner"`
	Assignee     string           `json:"assignee"`
	Labels       []string         `json:"labels"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
	ClosedAt     *string          `json:"closed_at"`
	Ephemeral    bool             `json:"ephemeral"`
	Wisp         bool             `json:"wisp"`
	Dependencies []BeadDependency `json:"dependencies"`
}

func (r *BeadsReader) readIssuesJSONL() ([]Bead, error) {
	issuesPath := filepath.Join(r.beadsDir, "issues.jsonl")
	file, err := os.Open(issuesPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	beadsOut := make([]Bead, 0, 128)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry issueJSONL
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		if entry.ID == "" {
			continue
		}

		bead := Bead{
			ID:           entry.ID,
			Title:        entry.Title,
			Description:  entry.Description,
			Status:       entry.Status,
			Priority:     entry.Priority,
			Type:         entry.Type,
			Owner:        entry.Owner,
			Assignee:     entry.Assignee,
			Labels:       entry.Labels,
			Ephemeral:    entry.Ephemeral || entry.Wisp,
			Dependencies: entry.Dependencies,
		}

		if entry.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, entry.CreatedAt); err == nil {
				bead.CreatedAt = t
			}
		}
		if entry.UpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, entry.UpdatedAt); err == nil {
				bead.UpdatedAt = t
			}
		}
		if entry.ClosedAt != nil && *entry.ClosedAt != "" {
			if t, err := time.Parse(time.RFC3339, *entry.ClosedAt); err == nil {
				bead.ClosedAt = &t
			}
		}

		beadsOut = append(beadsOut, bead)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(beadsOut) == 0 {
		return nil, fmt.Errorf("no issues loaded from %s", issuesPath)
	}
	return beadsOut, nil
}

func (r *BeadsReader) beadsCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	cmd, cancel := command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+r.beadsDir)
	return cmd, cancel
}

// ListAgents returns all available agents for assignment.
func (r *BeadsReader) ListAgents() ([]string, error) {
	// List all tmux sessions
	cmd, cancel := command("tmux", "list-sessions", "-F", "#{session_name}")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var agents []string
	sessions := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, session := range sessions {
		if session == "" {
			continue
		}

		var agent string
		if strings.HasPrefix(session, "hq-") {
			role := strings.TrimPrefix(session, "hq-")
			agent = role + "/"
		} else if strings.HasPrefix(session, "gt-") {
			parts := strings.SplitN(session, "-", 4)
			if len(parts) < 3 {
				continue
			}
			rig := parts[1]

			if parts[2] == "crew" && len(parts) >= 4 {
				agent = fmt.Sprintf("%s/crew/%s", rig, parts[3])
			} else if parts[2] == "witness" || parts[2] == "refinery" {
				agent = fmt.Sprintf("%s/%s/", rig, parts[2])
			} else {
				agent = fmt.Sprintf("%s/polecats/%s", rig, parts[2])
			}
		} else {
			continue
		}

		agents = append(agents, agent)
	}

	return agents, nil
}
