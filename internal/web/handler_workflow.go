package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// WorkflowPageData is the data passed to the workflow template.
type WorkflowPageData struct {
	Title      string
	ActivePage string
}

// HookStatus represents the current hook status for an agent.
type HookStatus struct {
	Actor       string `json:"actor"`
	Role        string `json:"role"`
	HasWork     bool   `json:"has_work"`
	WorkType    string `json:"work_type,omitempty"`    // "molecule", "mail", "none"
	WorkID      string `json:"work_id,omitempty"`      // ID of hooked work
	WorkTitle   string `json:"work_title,omitempty"`   // Title/subject of hooked work
	RawOutput   string `json:"raw_output"`             // Full output for display
}

type hookStatusJSON struct {
	Target           string `json:"target"`
	Role             string `json:"role"`
	HasWork          bool   `json:"has_work"`
	AttachedMolecule string `json:"attached_molecule,omitempty"`
	PinnedBead       *Bead  `json:"pinned_bead,omitempty"`
	NextAction       string `json:"next_action,omitempty"`
}

// ReadyIssue represents an issue with no blockers.
type ReadyIssue struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
	Type     string `json:"type"`
	Title    string `json:"title"`
}

// handleWorkflow serves the workflow page.
func (h *GUIHandler) handleWorkflow(w http.ResponseWriter, r *http.Request) {
	data := WorkflowPageData{
		Title:      "Workflow",
		ActivePage: "workflow",
	}
	h.renderTemplate(w, "workflow.html", data)
}

// handleAPIWorkflowHook returns the current hook status.
func (h *GUIHandler) handleAPIWorkflowHook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Use stale-while-revalidate (short TTL for hook status)
	cached := h.cache.GetStaleOrRefresh("workflow_hook", 5*time.Second, func() interface{} {
		return h.fetchHookStatus()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	status := h.fetchHookStatus()
	h.cache.Set("workflow_hook", status, 5*time.Second)
	json.NewEncoder(w).Encode(status)
}

// fetchHookStatus gets hook status from gt hook.
func (h *GUIHandler) fetchHookStatus() HookStatus {
	cmd, cancel := command("gt", "hook", "--json")
	defer cancel()
	output, err := cmd.Output()
	if err == nil {
		var parsed hookStatusJSON
		if json.Unmarshal(output, &parsed) == nil {
			status := HookStatus{
				Actor:     parsed.Target,
				Role:      parsed.Role,
				HasWork:   parsed.HasWork,
				WorkType:  "none",
				RawOutput: parsed.NextAction,
			}

			if parsed.PinnedBead != nil {
				status.WorkID = parsed.PinnedBead.ID
				status.WorkTitle = parsed.PinnedBead.Title
				if parsed.PinnedBead.Type != "" {
					status.WorkType = parsed.PinnedBead.Type
				}
			}

			if parsed.AttachedMolecule != "" {
				status.WorkType = "molecule"
			} else if status.WorkType == "none" && status.HasWork {
				status.WorkType = "hooked"
			}

			if status.RawOutput == "" {
				status.RawOutput = string(output)
			}

			return status
		}
	}

	cmd, cancel = command("gt", "hook")
	defer cancel()
	output, err = cmd.Output()
	if err != nil {
		return HookStatus{
			HasWork:   false,
			RawOutput: "Error: " + err.Error(),
		}
	}
	return parseHookOutput(string(output))
}

// parseHookOutput parses the gt hook command output.
func parseHookOutput(output string) HookStatus {
	status := HookStatus{
		RawOutput: output,
		HasWork:   false,
		WorkType:  "none",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse actor line: "🪝 Hook Status: TerraNomadicCity/crew/Myrtle"
		if strings.HasPrefix(line, "🪝 Hook Status:") {
			status.Actor = strings.TrimSpace(strings.TrimPrefix(line, "🪝 Hook Status:"))
		}

		// Parse role line
		if strings.HasPrefix(line, "Role:") {
			status.Role = strings.TrimSpace(strings.TrimPrefix(line, "Role:"))
		}

		// Check for "Nothing on hook"
		if strings.Contains(line, "Nothing on hook") {
			status.HasWork = false
			status.WorkType = "none"
		}

		// Check for molecule: "📦 Molecule: hq-mol-xxx"
		if strings.HasPrefix(line, "📦 Molecule:") {
			status.HasWork = true
			status.WorkType = "molecule"
			status.WorkID = strings.TrimSpace(strings.TrimPrefix(line, "📦 Molecule:"))
		}

		// Check for mail: "📬 Mail: hq-xxx"
		if strings.HasPrefix(line, "📬 Mail:") || strings.HasPrefix(line, "📧 Mail:") {
			status.HasWork = true
			status.WorkType = "mail"
			status.WorkID = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "📬 Mail:"), "📧 Mail:"))
		}
	}

	return status
}

// handleAPIWorkflowReady returns issues with no blockers.
func (h *GUIHandler) handleAPIWorkflowReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Use stale-while-revalidate
	cached := h.cache.GetStaleOrRefresh("workflow_ready", 15*time.Second, func() interface{} {
		return h.fetchReadyIssues()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchReadyIssues()
	h.cache.Set("workflow_ready", result, 15*time.Second)
	json.NewEncoder(w).Encode(result)
}

// fetchReadyIssues gets ready issues from bd ready.
func (h *GUIHandler) fetchReadyIssues() map[string]interface{} {
	cmd, cancel := command("bd", "ready")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{
			"issues": []interface{}{},
			"error":  err.Error(),
		}
	}
	return map[string]interface{}{
		"issues": parseReadyOutput(string(output)),
	}
}

// parseReadyOutput parses the bd ready command output.
// Format: "1. [● P1] [bug] te-0rr: SSH server accepts any password"
func parseReadyOutput(output string) []ReadyIssue {
	var issues []ReadyIssue
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip header and empty lines
		if line == "" || strings.HasPrefix(line, "📋") {
			continue
		}

		// Parse issue line: "1. [● P1] [bug] te-0rr: Title"
		if len(line) > 3 && line[0] >= '0' && line[0] <= '9' {
			issue := ReadyIssue{}

			// Extract priority
			if idx := strings.Index(line, "[● P"); idx != -1 {
				if len(line) > idx+5 {
					pChar := line[idx+4]
					if pChar >= '1' && pChar <= '4' {
						issue.Priority = int(pChar - '0')
					}
				}
			}

			// Extract type: look for [bug], [task], [feature], [doc]
			for _, t := range []string{"bug", "task", "feature", "doc"} {
				if strings.Contains(line, "["+t+"]") {
					issue.Type = t
					break
				}
			}

			// Extract ID and title: after type bracket, format is "id: title"
			// Find the ID pattern (letters-letters or letters-alphanum)
			parts := strings.SplitN(line, "]", 3)
			if len(parts) >= 3 {
				rest := strings.TrimSpace(parts[2])
				if colonIdx := strings.Index(rest, ":"); colonIdx != -1 {
					issue.ID = strings.TrimSpace(rest[:colonIdx])
					issue.Title = strings.TrimSpace(rest[colonIdx+1:])
				}
			}

			if issue.ID != "" {
				issues = append(issues, issue)
			}
		}
	}

	return issues
}

// handleAPIActivity returns recent git commits and activity.
// Kept for backwards compatibility but also used by workflow page.
func (h *GUIHandler) handleAPIActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Use stale-while-revalidate
	cached := h.cache.GetStaleOrRefresh("workflow_activity", 30*time.Second, func() interface{} {
		return h.fetchActivity()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchActivity()
	h.cache.Set("workflow_activity", result, 30*time.Second)
	json.NewEncoder(w).Encode(result)
}

// fetchActivity gets recent git commits.
func (h *GUIHandler) fetchActivity() map[string]interface{} {
	// Get recent commits from the current rig's repo
	rigDir := "/home/shisui/gt"

	// Check if GT_ROOT is set
	if gtRoot, cancel := command("gt", "env", "GT_ROOT"); gtRoot != nil {
		defer cancel()
		if out, err := gtRoot.Output(); err == nil {
			rigDir = strings.TrimSpace(string(out))
		}
	}

	cmd, cancel := command("git", "log", "--oneline", "-20", "--format=%h|%s|%cr|%an")
	defer cancel()
	cmd.Dir = rigDir
	output, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{
			"commits": []interface{}{},
			"error":   err.Error(),
		}
	}

	var commits []map[string]string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			commits = append(commits, map[string]string{
				"hash":    parts[0],
				"message": parts[1],
				"age":     parts[2],
				"author":  parts[3],
			})
		}
	}

	return map[string]interface{}{
		"commits": commits,
	}
}
