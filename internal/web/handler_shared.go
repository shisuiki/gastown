package web

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/workspace"
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
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil
	}

	manager := rig.NewManager(townRoot, rigsConfig, git.NewGit(townRoot))
	found, err := manager.DiscoverRigs()
	if err != nil {
		return nil
	}

	rigs := make([]RigStatus, 0, len(found))
	for _, item := range found {
		rigs = append(rigs, RigStatus{
			Name:       item.Name,
			Path:       item.Path,
			Polecats:   len(item.Polecats),
			Crew:       len(item.Crew),
			HasWitness: item.HasWitness,
		})
	}
	return rigs
}
