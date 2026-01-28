package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleAPIBeads returns a comprehensive list of beads with filtering.
func (h *GUIHandler) handleAPIBeads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"beads": []Bead{},
		})
		return
	}

	filter := BeadFilter{
		Status:           r.URL.Query().Get("status"),
		Type:             r.URL.Query().Get("type"),
		Assignee:         r.URL.Query().Get("assignee"),
		IncludeEphemeral: r.URL.Query().Get("ephemeral") == "true",
	}

	// Default: exclude system types unless specifically requested
	if filter.Type == "" && r.URL.Query().Get("include_system") != "true" {
		filter.ExcludeTypes = []string{"agent", "molecule", "gate", "event", "message"}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		filter.Limit, _ = strconv.Atoi(limitStr)
	}
	if filter.Limit == 0 {
		filter.Limit = 200
	}

	beads, err := reader.ListBeads(filter)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"beads": []Bead{},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"beads": beads,
		"count": len(beads),
	})
}

// handleAPIBeadByID returns a single bead with full details.
func (h *GUIHandler) handleAPIBeadByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract bead ID from path: /api/beads/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/beads/")
	beadID := strings.Split(path, "/")[0]

	if beadID == "" {
		http.Error(w, "Bead ID required", http.StatusBadRequest)
		return
	}

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	bead, err := reader.GetBead(beadID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Bead not found: " + err.Error(),
		})
		return
	}

	// Get dependencies
	deps, _ := reader.GetBeadDependencies(beadID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"bead":         bead,
		"dependencies": deps,
	})
}

// handleAPIBeadSearch searches beads by text.
func (h *GUIHandler) handleAPIBeadSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	if query == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Search query required",
			"beads": []Bead{},
		})
		return
	}

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"beads": []Bead{},
		})
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}

	beads, err := reader.SearchBeads(query, limit)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"beads": []Bead{},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"beads": beads,
		"count": len(beads),
		"query": query,
	})
}

// handleAPIBeadStats returns statistics about beads.
func (h *GUIHandler) handleAPIBeadStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	stats, err := reader.GetBeadStats()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(stats)
}

// handleAPIAllAgentHooks returns hook status for all active agents.
func (h *GUIHandler) handleAPIAllAgentHooks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cached := h.cache.GetStaleOrRefresh("agent_hooks", 10*time.Second, func() interface{} {
		return h.fetchAgentHooks()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	result := h.fetchAgentHooks()
	h.cache.Set("agent_hooks", result, 10*time.Second)
	json.NewEncoder(w).Encode(result)
}

func (h *GUIHandler) fetchAgentHooks() map[string]interface{} {
	reader, err := NewBeadsReader("")
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
			"hooks": []AgentHook{},
		}
	}

	hooks, err := reader.GetAllAgentHooks()
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
			"hooks": []AgentHook{},
		}
	}

	return map[string]interface{}{
		"hooks": hooks,
		"count": len(hooks),
	}
}

// handleAPIConvoyBeads returns the beads tracked by a convoy.
func (h *GUIHandler) handleAPIConvoyBeads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract convoy ID from path: /api/convoy/beads/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/convoy/beads/")
	convoyID := strings.Split(path, "/")[0]

	if convoyID == "" {
		http.Error(w, "Convoy ID required", http.StatusBadRequest)
		return
	}

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"beads": []Bead{},
		})
		return
	}

	beads, err := reader.GetConvoyTrackedIssues(convoyID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"beads": []Bead{},
		})
		return
	}

	// Calculate progress
	completed := 0
	for _, b := range beads {
		if b.Status == "closed" {
			completed++
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"beads":     beads,
		"count":     len(beads),
		"completed": completed,
		"total":     len(beads),
		"progress":  float64(completed) / float64(max(len(beads), 1)) * 100,
	})
}

// handleAPIAvailableAgents returns list of agents for assignment.
func (h *GUIHandler) handleAPIAvailableAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  err.Error(),
			"agents": []string{},
		})
		return
	}

	agents, err := reader.ListAgents()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  err.Error(),
			"agents": []string{},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

// handleAPIBeadAction handles bead operations (sling, close, update, etc.)
func (h *GUIHandler) handleAPIBeadAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Action   string `json:"action"`   // "sling", "close", "reopen", "update"
		BeadID   string `json:"bead_id"`
		Target   string `json:"target,omitempty"`   // For sling: target agent
		Status   string `json:"status,omitempty"`   // For update
		Assignee string `json:"assignee,omitempty"` // For update
		Priority int    `json:"priority,omitempty"` // For update
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON: " + err.Error(),
		})
		return
	}

	if req.BeadID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "bead_id is required",
		})
		return
	}

	var output []byte
	var err error
	run := func(name string, args ...string) ([]byte, error) {
		cmd, cancel := command(name, args...)
		defer cancel()
		return cmd.CombinedOutput()
	}

	switch req.Action {
	case "sling":
		if req.Target == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "target is required for sling action",
			})
			return
		}
		output, err = run("gt", "sling", req.BeadID, req.Target)

	case "close":
		output, err = run("bd", "close", req.BeadID)

	case "reopen":
		output, err = run("bd", "reopen", req.BeadID)

	case "update":
		args := []string{"update", req.BeadID}
		if req.Status != "" {
			args = append(args, "--status="+req.Status)
		}
		if req.Assignee != "" {
			args = append(args, "--assignee="+req.Assignee)
		}
		if req.Priority > 0 {
			args = append(args, "--priority="+strconv.Itoa(req.Priority))
		}
		output, err = run("bd", args...)

	case "unsling":
		output, err = run("gt", "unsling")

	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Unknown action: " + req.Action,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": err == nil,
		"output":  string(output),
		"error":   err != nil,
	})
}

// handleAPICreateBeadV2 creates a new bead with more options.
func (h *GUIHandler) handleAPICreateBeadV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description,omitempty"`
		Type        string   `json:"type,omitempty"` // task, bug, feature, etc.
		Priority    int      `json:"priority,omitempty"`
		Assignee    string   `json:"assignee,omitempty"`
		Labels      []string `json:"labels,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON: " + err.Error(),
		})
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "title is required",
		})
		return
	}

	workDir := webWorkDir()
	args := webBeadsArgs("create", "--json", "--title="+req.Title)

	issueType := normalizeIssueType(req.Type)
	if issueType != "" {
		args = append(args, "--type="+issueType)
	}
	if req.Description != "" {
		args = append(args, "--description="+req.Description)
	}
	if req.Priority > 0 {
		args = append(args, "--priority="+strconv.Itoa(req.Priority))
	}
	if req.Assignee != "" {
		args = append(args, "--assignee="+req.Assignee)
	}

	labels := make([]string, 0, len(req.Labels))
	for _, label := range req.Labels {
		label = strings.TrimSpace(label)
		if label != "" {
			labels = append(labels, label)
		}
	}
	if len(labels) > 0 {
		args = append(args, "--labels="+strings.Join(labels, ","))
	}

	cmd, cancel := command("bd", args...)
	defer cancel()
	cmd.Dir = workDir
	cmd.Env = webBeadsEnv(workDir)
	output, err := cmd.CombinedOutput()

	outMsg, beadID := parseCreateOutput(output)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": err == nil,
		"output":  outMsg,
		"bead_id": beadID,
		"error": func() string {
			if err == nil {
				return ""
			}
			return err.Error()
		}(),
	})
}

// handleAPIBeadDetailFast returns detailed bead info using direct DB access.
// This is the fast version that replaces the CLI-based handler.
func (h *GUIHandler) handleAPIBeadDetailFast(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract ID from path: /api/bead/te-xxx
	id := strings.TrimPrefix(r.URL.Path, "/api/bead/")
	if id == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing bead ID"})
		return
	}

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    id,
			"error": err.Error(),
		})
		return
	}

	bead, err := reader.GetBead(id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    id,
			"error": "Bead not found: " + err.Error(),
		})
		return
	}

	// Get dependencies
	deps, _ := reader.GetBeadDependencies(id)

	// Return in format compatible with detail page
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":           bead.ID,
		"title":        bead.Title,
		"type":         bead.Type,
		"priority":     bead.Priority,
		"status":       bead.Status,
		"owner":        bead.Owner,
		"assignee":     bead.Assignee,
		"description":  bead.Description,
		"labels":       bead.Labels,
		"created":      bead.CreatedAt.Format("2006-01-02 15:04"),
		"updated":      bead.UpdatedAt.Format("2006-01-02 15:04"),
		"dependencies": deps,
	})
}

// handleAPIConvoyDetailFast returns detailed convoy info using direct DB access.
func (h *GUIHandler) handleAPIConvoyDetailFast(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract ID from path: /api/convoy/hq-cv-xxx
	id := strings.TrimPrefix(r.URL.Path, "/api/convoy/")
	// Remove "beads/" suffix if present (different endpoint)
	if strings.HasPrefix(id, "beads/") {
		h.handleAPIConvoyBeads(w, r)
		return
	}

	if id == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing convoy ID"})
		return
	}

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    id,
			"error": err.Error(),
		})
		return
	}

	// Get convoy bead
	convoy, err := reader.GetBead(id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    id,
			"error": "Convoy not found: " + err.Error(),
		})
		return
	}

	// Get tracked issues
	trackedIssues, _ := reader.GetConvoyTrackedIssues(id)

	// Calculate progress
	completed := 0
	for _, issue := range trackedIssues {
		if issue.Status == "closed" {
			completed++
		}
	}
	total := len(trackedIssues)

	progress := "0/0"
	if total > 0 {
		progress = strconv.Itoa(completed) + "/" + strconv.Itoa(total)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             convoy.ID,
		"title":          convoy.Title,
		"status":         convoy.Status,
		"progress":       progress,
		"completed":      completed,
		"total":          total,
		"created":        convoy.CreatedAt.Format("2006-01-02 15:04"),
		"tracked_issues": trackedIssues,
	})
}

// max returns the larger of a or b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
