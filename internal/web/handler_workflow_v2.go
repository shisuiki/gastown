package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
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

	reader, err := NewBeadsReader("")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"hooks": []AgentHook{},
		})
		return
	}

	hooks, err := reader.GetAllAgentHooks()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
			"hooks": []AgentHook{},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"hooks": hooks,
		"count": len(hooks),
	})
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

	var cmd *exec.Cmd
	var output []byte
	var err error

	switch req.Action {
	case "sling":
		if req.Target == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "target is required for sling action",
			})
			return
		}
		cmd = exec.Command("gt", "sling", req.BeadID, req.Target)
		output, err = cmd.CombinedOutput()

	case "close":
		cmd = exec.Command("bd", "close", req.BeadID)
		output, err = cmd.CombinedOutput()

	case "reopen":
		cmd = exec.Command("bd", "reopen", req.BeadID)
		output, err = cmd.CombinedOutput()

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
		cmd = exec.Command("bd", args...)
		output, err = cmd.CombinedOutput()

	case "unsling":
		cmd = exec.Command("gt", "unsling")
		output, err = cmd.CombinedOutput()

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

	if req.Title == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "title is required",
		})
		return
	}

	args := []string{"create", req.Title}

	if req.Description != "" {
		args = append(args, "--description="+req.Description)
	}
	if req.Type != "" {
		args = append(args, "--type="+req.Type)
	}
	if req.Priority > 0 {
		args = append(args, "--priority="+strconv.Itoa(req.Priority))
	}
	if req.Assignee != "" {
		args = append(args, "--assignee="+req.Assignee)
	}
	for _, label := range req.Labels {
		args = append(args, "--label="+label)
	}

	cmd := exec.Command("bd", args...)
	output, err := cmd.CombinedOutput()

	// Try to extract the created bead ID from output
	var beadID string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Created") || strings.Contains(line, "created") {
			// Look for ID pattern like "hq-xxx" or "te-xxx"
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, "-") && len(part) > 3 && len(part) < 20 {
					beadID = strings.Trim(part, ":")
					break
				}
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": err == nil,
		"output":  string(output),
		"bead_id": beadID,
		"error":   err != nil,
	})
}

// max returns the larger of a or b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
