package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
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

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "title is required",
		})
		return
	}

	workDir := webTownRoot()
	convoyID := fmt.Sprintf("hq-cv-%s", generateShortID())
	description := fmt.Sprintf("Convoy tracking %d issues", len(req.Issues))

	args := webBeadsArgs(
		"create",
		"--type=convoy",
		"--id="+convoyID,
		"--title="+req.Title,
		"--description="+description,
	)
	if beads.NeedsForceForID(convoyID) {
		args = append(args, "--force")
	}

	cmd, cancel := command("bd", args...)
	defer cancel()
	cmd.Dir = workDir
	cmd.Env = webBeadsEnv(workDir)
	output, err := cmd.CombinedOutput()

	if err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	trackedCount := 0
	var depErrors []string
	for _, issueID := range req.Issues {
		issueID = strings.TrimSpace(issueID)
		if issueID == "" {
			continue
		}
		trackedCount++
		depArgs := webBeadsArgs("dep", "add", convoyID, issueID, "--type=tracks")
		depCmd, depCancel := command("bd", depArgs...)
		depCmd.Dir = workDir
		depCmd.Env = webBeadsEnv(workDir)
		depOutput, depErr := depCmd.CombinedOutput()
		depCancel()
		if depErr != nil {
			msg := strings.TrimSpace(string(depOutput))
			if msg == "" {
				msg = depErr.Error()
			}
			depErrors = append(depErrors, fmt.Sprintf("%s: %s", issueID, msg))
		}
	}

	outputMsg := fmt.Sprintf("Created convoy %s", convoyID)
	if trackedCount > 0 {
		outputMsg = fmt.Sprintf("%s tracking %d issue(s)", outputMsg, trackedCount)
	}
	if len(depErrors) > 0 {
		outputMsg = outputMsg + "\nWarnings:\n" + strings.Join(depErrors, "\n")
	}

	if outputMsg == "" {
		outputMsg = strings.TrimSpace(string(output))
	}

	json.NewEncoder(w).Encode(ActionResponse{
		Success: true,
		Output:  outputMsg,
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

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Error:   "title is required",
		})
		return
	}

	// Default type to task
	issueType := normalizeIssueType(req.Type)

	// Default priority to 2
	priority := req.Priority
	if priority < 1 || priority > 4 {
		priority = 2
	}

	workDir := webWorkDir()
	args := webBeadsArgs(
		"create",
		"--json",
		"--title="+req.Title,
		"--type="+issueType,
		"--priority="+strconv.Itoa(priority),
	)
	if req.Body != "" {
		args = append(args, "--description="+req.Body)
	}

	cmd, cancel := command("bd", args...)
	defer cancel()
	cmd.Dir = workDir
	cmd.Env = webBeadsEnv(workDir)
	output, err := cmd.CombinedOutput()

	if err != nil {
		json.NewEncoder(w).Encode(ActionResponse{
			Success: false,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	outStr, _ := parseCreateOutput(output)
	json.NewEncoder(w).Encode(ActionResponse{
		Success: true,
		Output:  outStr,
	})
}
