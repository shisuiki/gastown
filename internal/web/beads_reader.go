package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BeadsReader provides direct access to beads SQLite database via sqlite3 CLI.
type BeadsReader struct {
	dbPath   string
	townRoot string
}

// Bead represents a bead from the database.
type Bead struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Type        string    `json:"issue_type"`
	Owner       string    `json:"owner,omitempty"`
	Assignee    string    `json:"assignee,omitempty"`
	Labels      []string  `json:"labels,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	Ephemeral   bool      `json:"ephemeral,omitempty"`
}

// BeadDependency represents a dependency between beads.
type BeadDependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"` // "blocks", "tracks", "parent-child"
}

// AgentHook represents an agent's hook status.
type AgentHook struct {
	Agent     string `json:"agent"`      // e.g., "TerraNomadicCity/crew/Myrtle"
	Role      string `json:"role"`       // e.g., "crew"
	HasWork   bool   `json:"has_work"`
	WorkType  string `json:"work_type"`  // "molecule", "mail", "none"
	WorkID    string `json:"work_id,omitempty"`
	WorkTitle string `json:"work_title,omitempty"`
}

// NewBeadsReader creates a BeadsReader for the given town root.
func NewBeadsReader(townRoot string) (*BeadsReader, error) {
	if townRoot == "" {
		// Try to find from environment or default
		townRoot = os.Getenv("GT_ROOT")
		if townRoot == "" {
			home, _ := os.UserHomeDir()
			townRoot = filepath.Join(home, "gt")
		}
	}

	dbPath := filepath.Join(townRoot, ".beads", "beads.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("beads database not found at %s", dbPath)
	}

	return &BeadsReader{
		dbPath:   dbPath,
		townRoot: townRoot,
	}, nil
}

// query executes a SQL query and returns JSON results.
func (r *BeadsReader) query(sql string) ([]byte, error) {
	// #nosec G204 -- sqlite3 path is from trusted config
	cmd := exec.Command("sqlite3", "-json", r.dbPath, sql)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite3 error: %s", stderr.String())
	}
	return stdout.Bytes(), nil
}

// ListBeads returns beads matching the given filters.
func (r *BeadsReader) ListBeads(filter BeadFilter) ([]Bead, error) {
	query := `SELECT id, title, COALESCE(description, ''), status, priority,
	          COALESCE(issue_type, 'task'), COALESCE(owner, ''), COALESCE(assignee, ''),
	          COALESCE(labels, '[]'), created_at, updated_at, closed_at, COALESCE(ephemeral, 0)
	          FROM issues WHERE 1=1`

	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = '%s'", escapeSQL(filter.Status))
	}
	if filter.Type != "" {
		query += fmt.Sprintf(" AND issue_type = '%s'", escapeSQL(filter.Type))
	}
	if filter.Assignee != "" {
		query += fmt.Sprintf(" AND assignee = '%s'", escapeSQL(filter.Assignee))
	}
	if len(filter.ExcludeTypes) > 0 {
		types := make([]string, len(filter.ExcludeTypes))
		for i, t := range filter.ExcludeTypes {
			types[i] = "'" + escapeSQL(t) + "'"
		}
		query += fmt.Sprintf(" AND (issue_type IS NULL OR issue_type NOT IN (%s))", strings.Join(types, ","))
	}
	if !filter.IncludeEphemeral {
		query += " AND (ephemeral IS NULL OR ephemeral = 0)"
	}
	if filter.Priority > 0 {
		query += fmt.Sprintf(" AND priority >= %d", filter.Priority)
	}

	query += " ORDER BY priority DESC, updated_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	output, err := r.query(query)
	if err != nil {
		return nil, err
	}

	return r.parseBeadsJSON(output)
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

// parseBeadsJSON parses sqlite3 JSON output into Bead structs.
func (r *BeadsReader) parseBeadsJSON(data []byte) ([]Bead, error) {
	if len(data) == 0 {
		return []Bead{}, nil
	}

	var rows []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"COALESCE(description, '')"`
		Status      string `json:"status"`
		Priority    int    `json:"priority"`
		Type        string `json:"COALESCE(issue_type, 'task')"`
		Owner       string `json:"COALESCE(owner, '')"`
		Assignee    string `json:"COALESCE(assignee, '')"`
		Labels      string `json:"COALESCE(labels, '[]')"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
		ClosedAt    string `json:"closed_at"`
		Ephemeral   int    `json:"COALESCE(ephemeral, 0)"`
	}

	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}

	beads := make([]Bead, len(rows))
	for i, row := range rows {
		beads[i] = Bead{
			ID:          row.ID,
			Title:       row.Title,
			Description: row.Description,
			Status:      row.Status,
			Priority:    row.Priority,
			Type:        row.Type,
			Owner:       row.Owner,
			Assignee:    row.Assignee,
			Ephemeral:   row.Ephemeral == 1,
		}
		beads[i].CreatedAt, _ = time.Parse(time.RFC3339, row.CreatedAt)
		beads[i].UpdatedAt, _ = time.Parse(time.RFC3339, row.UpdatedAt)
		if row.ClosedAt != "" {
			t, _ := time.Parse(time.RFC3339, row.ClosedAt)
			beads[i].ClosedAt = &t
		}
		if row.Labels != "" && row.Labels != "[]" {
			json.Unmarshal([]byte(row.Labels), &beads[i].Labels)
		}
	}

	return beads, nil
}

// GetBead returns a single bead by ID.
func (r *BeadsReader) GetBead(id string) (*Bead, error) {
	query := fmt.Sprintf(`SELECT id, title, COALESCE(description, ''), status, priority,
	          COALESCE(issue_type, 'task'), COALESCE(owner, ''), COALESCE(assignee, ''),
	          COALESCE(labels, '[]'), created_at, updated_at, closed_at, COALESCE(ephemeral, 0)
	          FROM issues WHERE id = '%s'`, escapeSQL(id))

	output, err := r.query(query)
	if err != nil {
		return nil, err
	}

	beads, err := r.parseBeadsJSON(output)
	if err != nil {
		return nil, err
	}
	if len(beads) == 0 {
		return nil, fmt.Errorf("bead not found: %s", id)
	}

	return &beads[0], nil
}

// GetConvoyTrackedIssues returns the issues tracked by a convoy.
func (r *BeadsReader) GetConvoyTrackedIssues(convoyID string) ([]Bead, error) {
	// Get tracked issue IDs from dependencies table
	query := fmt.Sprintf(`SELECT depends_on_id FROM dependencies
	                       WHERE issue_id = '%s' AND type = 'tracks'`, escapeSQL(convoyID))

	output, err := r.query(query)
	if err != nil {
		return nil, err
	}

	var deps []struct {
		DependsOnID string `json:"depends_on_id"`
	}
	if len(output) > 0 {
		json.Unmarshal(output, &deps)
	}

	if len(deps) == 0 {
		return []Bead{}, nil
	}

	// Collect issue IDs
	issueIDs := make([]string, 0, len(deps))
	for _, dep := range deps {
		id := dep.DependsOnID
		// Handle external references
		if strings.HasPrefix(id, "external:") {
			parts := strings.SplitN(id, ":", 3)
			if len(parts) == 3 {
				id = parts[2]
			}
		}
		issueIDs = append(issueIDs, "'"+escapeSQL(id)+"'")
	}

	// Fetch the actual issues
	query = fmt.Sprintf(`SELECT id, title, COALESCE(description, ''), status, priority,
	          COALESCE(issue_type, 'task'), COALESCE(owner, ''), COALESCE(assignee, ''),
	          COALESCE(labels, '[]'), created_at, updated_at, closed_at, COALESCE(ephemeral, 0)
	          FROM issues WHERE id IN (%s)`, strings.Join(issueIDs, ","))

	output, err = r.query(query)
	if err != nil {
		return nil, err
	}

	return r.parseBeadsJSON(output)
}

// GetAllAgentHooks returns hook status for all active agents.
func (r *BeadsReader) GetAllAgentHooks() ([]AgentHook, error) {
	// List all tmux sessions to find active agents
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
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

	// Try to get hook info by running gt hook with actor context
	cmd := exec.Command("gt", "hook", "show", agent)
	output, err := cmd.Output()
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
	query := fmt.Sprintf(`SELECT issue_id, depends_on_id, type FROM dependencies
	                       WHERE issue_id = '%s' OR depends_on_id = '%s'`,
		escapeSQL(beadID), escapeSQL(beadID))

	output, err := r.query(query)
	if err != nil {
		return nil, err
	}

	var deps []BeadDependency
	if len(output) > 0 {
		json.Unmarshal(output, &deps)
	}

	return deps, nil
}

// GetBeadStats returns statistics about beads.
func (r *BeadsReader) GetBeadStats() (map[string]int, error) {
	stats := make(map[string]int)

	// Count by status
	output, err := r.query(`SELECT status, COUNT(*) as count FROM issues GROUP BY status`)
	if err == nil && len(output) > 0 {
		var rows []struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		json.Unmarshal(output, &rows)
		for _, row := range rows {
			stats["status_"+row.Status] = row.Count
		}
	}

	// Count by type
	output, err = r.query(`SELECT COALESCE(issue_type, 'task') as type, COUNT(*) as count FROM issues GROUP BY issue_type`)
	if err == nil && len(output) > 0 {
		var rows []struct {
			Type  string `json:"type"`
			Count int    `json:"count"`
		}
		json.Unmarshal(output, &rows)
		for _, row := range rows {
			stats["type_"+row.Type] = row.Count
		}
	}

	// Total count
	output, _ = r.query(`SELECT COUNT(*) as count FROM issues`)
	if len(output) > 0 {
		var rows []struct {
			Count int `json:"count"`
		}
		json.Unmarshal(output, &rows)
		if len(rows) > 0 {
			stats["total"] = rows[0].Count
		}
	}

	return stats, nil
}

// SearchBeads searches beads by text in title and description.
func (r *BeadsReader) SearchBeads(searchQuery string, limit int) ([]Bead, error) {
	escapedQuery := escapeSQL(searchQuery)
	query := fmt.Sprintf(`SELECT id, title, COALESCE(description, ''), status, priority,
	          COALESCE(issue_type, 'task'), COALESCE(owner, ''), COALESCE(assignee, ''),
	          COALESCE(labels, '[]'), created_at, updated_at, closed_at, COALESCE(ephemeral, 0)
	          FROM issues
	          WHERE (title LIKE '%%%s%%' OR description LIKE '%%%s%%' OR id LIKE '%%%s%%')
	          AND (ephemeral IS NULL OR ephemeral = 0)
	          ORDER BY updated_at DESC`,
		escapedQuery, escapedQuery, escapedQuery)

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	output, err := r.query(query)
	if err != nil {
		return nil, err
	}

	return r.parseBeadsJSON(output)
}

// ListAgents returns all available agents for assignment.
func (r *BeadsReader) ListAgents() ([]string, error) {
	// List all tmux sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
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

// escapeSQL escapes single quotes in SQL strings.
func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
