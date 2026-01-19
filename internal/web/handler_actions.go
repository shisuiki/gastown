package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ActionRequest represents a request to run a GT action.
type ActionRequest struct {
	Action string   `json:"action"`
	Args   []string `json:"args,omitempty"`
}

// ActionResponse represents the result of a GT action.
type ActionResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// handleAPIActions handles quick action requests.
func (h *GUIHandler) handleAPIActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "method not allowed",
		})
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	// Whitelist of allowed actions for security
	allowedActions := map[string][]string{
		"daemon-status": {"gt", "daemon", "status"},
		"daemon-start":  {"gt", "daemon", "start"},
		"daemon-stop":   {"gt", "daemon", "stop"},
		"rig-list":      {"gt", "rig", "list"},
		"convoy-list":   {"gt", "convoy", "list"},
		"mail-inbox":    {"gt", "mail", "inbox"},
		"hook-status":   {"gt", "hook"},
		"bd-ready":      {"bd", "ready"},
		"bd-list":       {"bd", "list"},
		"bd-sync":       {"bd", "sync"},
	}

	cmdArgs, ok := allowedActions[req.Action]
	if !ok {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "unknown action: " + req.Action,
		})
		return
	}

	// Append any additional args (for some actions)
	if len(req.Args) > 0 {
		cmdArgs = append(cmdArgs, req.Args...)
	}

	cmd, cancel := command(cmdArgs[0], cmdArgs[1:]...)
	defer cancel()
	output, err := cmd.CombinedOutput()

	if err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(ActionResponse{
		Success: true,
		Output:  string(output),
	})
}

// handleAPICreateConvoy handles convoy creation requests.
func (h *GUIHandler) handleAPICreateConvoy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "method not allowed",
		})
		return
	}

	var req struct {
		Title  string   `json:"title"`
		Issues []string `json:"issues,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	if req.Title == "" {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "title is required",
		})
		return
	}

	// Build command: gt convoy create <title> [issues...]
	args := []string{"convoy", "create", req.Title}
	if len(req.Issues) > 0 {
		args = append(args, req.Issues...)
	}

	cmd, cancel := command("gt", args...)
	defer cancel()
	output, err := cmd.CombinedOutput()

	if err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(ActionResponse{
		Success: true,
		Output:  string(output),
	})
}

// handleAPICreateBead handles bead creation requests.
func (h *GUIHandler) handleAPICreateBead(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "method not allowed",
		})
		return
	}

	var req struct {
		Title    string `json:"title"`
		Type     string `json:"type"`     // bug, task, feature, doc
		Priority int    `json:"priority"` // 1-4
		Body     string `json:"body,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	if req.Title == "" {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "title is required",
		})
		return
	}

	// Default type to task
	issueType := req.Type
	if issueType == "" {
		issueType = "task"
	}

	// Default priority to 2
	priority := req.Priority
	if priority < 1 || priority > 4 {
		priority = 2
	}

	// Build command: bd create -t <type> "<title>" -p <priority>
	args := []string{"create", "-t", issueType, req.Title, "-p", string(rune('0' + priority))}
	if req.Body != "" {
		args = append(args, "-b", req.Body)
	}

	cmd, cancel := command("bd", args...)
	defer cancel()
	output, err := cmd.CombinedOutput()

	if err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	// Extract the bead ID from output if possible
	outStr := strings.TrimSpace(string(output))
	json.NewEncoder(w).Encode(ActionResponse{
		Success: true,
		Output:  outStr,
	})
}
