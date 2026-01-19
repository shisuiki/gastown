package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BeadsReader provides access to beads via bd CLI commands.
type BeadsReader struct {
	townRoot string
}

// Bead represents a bead from the database.
type Bead struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	Type        string     `json:"issue_type"`
	Owner       string     `json:"owner,omitempty"`
	Assignee    string     `json:"assignee,omitempty"`
	Labels      []string   `json:"labels,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	Ephemeral   bool       `json:"ephemeral,omitempty"`
}

// BeadDependency represents a dependency between beads.
type BeadDependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"` // "blocks", "tracks", "parent-child"
}

// AgentHook represents an agent's hook status.
type AgentHook struct {
	Agent     string `json:"agent"`     // e.g., "TerraNomadicCity/crew/Myrtle"
	Role      string `json:"role"`      // e.g., "crew"
	HasWork   bool   `json:"has_work"`
	WorkType  string `json:"work_type"` // "molecule", "mail", "none"
	WorkID    string `json:"work_id,omitempty"`
	WorkTitle string `json:"work_title,omitempty"`
}

// NewBeadsReader creates a BeadsReader for the given town root.
func NewBeadsReader(townRoot string) (*BeadsReader, error) {
	if townRoot == "" {
		townRoot = os.Getenv("GT_ROOT")
		if townRoot == "" {
			home, _ := os.UserHomeDir()
			townRoot = filepath.Join(home, "gt")
		}
	}

	return &BeadsReader{
		townRoot: townRoot,
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

	cmd, cancel := command("bd", args...)
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd list failed: %w", err)
	}

	var beads []Bead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Filter out excluded types and ephemeral if needed
	filtered := make([]Bead, 0, len(beads))
	for _, b := range beads {
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
	}

	return filtered, nil
}

// GetBead returns a single bead by ID using bd show --json.
func (r *BeadsReader) GetBead(id string) (*Bead, error) {
	cmd, cancel := command("bd", "show", id, "--json")
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
	// Use bd show with --json to get the convoy and its dependents
	cmd, cancel := command("bd", "show", convoyID, "--json")
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
	// Use bd show --json to get dependencies
	cmd, cancel := command("bd", "show", beadID, "--json")
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

	// Get all beads with JSON
	cmd, cancel := command("bd", "list", "--json", "--limit=0")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return stats, err
	}

	var beads []Bead
	if err := json.Unmarshal(output, &beads); err != nil {
		return stats, err
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
	args := []string{"search", searchQuery, "--json"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", limit))
	}

	cmd, cancel := command("bd", args...)
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
