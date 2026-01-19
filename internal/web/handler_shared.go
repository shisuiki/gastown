package web

import (
	"encoding/json"
	"net/http"
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
	cmd, cancel := command("gt", args...)
	defer cancel()
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
	cmd, cancel := command("gt", "rig", "list", "--json")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var raw []struct {
		Name       string `json:"name"`
		Path       string `json:"path"`
		Polecats   int    `json:"polecats"`
		Crew       int    `json:"crew"`
		HasWitness bool   `json:"has_witness"`
	}

	if err := json.Unmarshal(output, &raw); err != nil {
		return nil
	}

	rigs := make([]RigStatus, 0, len(raw))
	for _, item := range raw {
		rigs = append(rigs, RigStatus{
			Name:       item.Name,
			Path:       item.Path,
			Polecats:   item.Polecats,
			Crew:       item.Crew,
			HasWitness: item.HasWitness,
		})
	}
	return rigs
}
