package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
)

// handleAPIRigs returns rigs status.
func (h *GUIHandler) handleAPIRigs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rigs := h.getRigs()
	json.NewEncoder(w).Encode(rigs)
}

// handleAPIConvoys returns convoys status.
func (h *GUIHandler) handleAPIConvoys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	convoys, err := h.fetcher.FetchConvoys()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(convoys)
}

// handleAPICommand executes a gt command.
func (h *GUIHandler) handleAPICommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Only allow safe gt commands
	allowedCommands := map[string]bool{
		"status": true, "rig": true, "convoy": true,
		"mail": true, "hook": true, "ready": true,
		"trail": true, "daemon": true, "bead": true,
		"agents": true, "polecat": true,
	}

	if !allowedCommands[req.Command] {
		http.Error(w, "Command not allowed", http.StatusForbidden)
		return
	}

	args := append([]string{req.Command}, req.Args...)
	cmd := exec.Command("gt", args...)
	output, err := cmd.CombinedOutput()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": err == nil,
		"output":  string(output),
		"error":   err != nil,
	})
}

// getRigs parses the gt rig list output.
func (h *GUIHandler) getRigs() []RigStatus {
	cmd := exec.Command("gt", "rig", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Parse text output (gt rig list doesn't have --json yet)
	// Format:
	//   Rigs in /path:
	//
	//     rigname
	//       Polecats: N  Crew: M
	//       Agents: [...]
	var rigs []RigStatus
	var currentRig *RigStatus
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Rigs in") || line == "" {
			continue
		}
		// Rig name: 2 spaces then name
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			name := strings.TrimSpace(line)
			if name != "" {
				rigs = append(rigs, RigStatus{Name: name})
				currentRig = &rigs[len(rigs)-1]
			}
		}
		// Rig details: 4 spaces then "Polecats: N  Crew: M"
		if strings.HasPrefix(line, "    ") && currentRig != nil {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Polecats:") {
				var polecats, crew int
				fmt.Sscanf(trimmed, "Polecats: %d  Crew: %d", &polecats, &crew)
				currentRig.Polecats = polecats
				currentRig.Crew = crew
			}
		}
	}
	return rigs
}
