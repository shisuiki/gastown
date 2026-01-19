package web

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/config"
)

// ConfigPageData is the data passed to the config template.
type ConfigPageData struct {
	Title      string
	ActivePage string
}

// handleConfig serves the config page.
func (h *GUIHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	data := ConfigPageData{
		Title:      "Config",
		ActivePage: "config",
	}
	h.renderTemplate(w, "config.html", data)
}

// ConfigResponse is the API response for town settings.
type ConfigResponse struct {
	DefaultAgent     string                        `json:"default_agent"`
	RoleAgents       map[string]string             `json:"role_agents"`
	RoleModels       map[string]*config.RoleModelConfig `json:"role_models,omitempty"`
	Agents           map[string]*config.RuntimeConfig `json:"agents"`
	AgentEmailDomain string                        `json:"agent_email_domain"`
}

// ConfigRequest is the API request for updating town settings.
type ConfigRequest struct {
	DefaultAgent     string                        `json:"default_agent"`
	RoleAgents       map[string]string             `json:"role_agents"`
	RoleModels       map[string]*config.RoleModelConfig `json:"role_models,omitempty"`
	Agents           map[string]*config.RuntimeConfig `json:"agents"`
	AgentEmailDomain string                        `json:"agent_email_domain"`
}

// handleAPIConfig handles GET and POST for /api/config.
func (h *GUIHandler) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleAPIConfigGet(w, r)
	case http.MethodPost:
		h.handleAPIConfigPost(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIConfigGet returns the current town settings.
func (h *GUIHandler) handleAPIConfigGet(w http.ResponseWriter, r *http.Request) {
	gtRoot := os.Getenv("GT_ROOT")
	if gtRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "Cannot determine GT_ROOT", http.StatusInternalServerError)
			return
		}
		gtRoot = filepath.Join(home, "gt")
	}

	settingsPath := config.TownSettingsPath(gtRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		log.Printf("Error loading town settings: %v", err)
		http.Error(w, "Failed to load config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := ConfigResponse{
		DefaultAgent:     settings.DefaultAgent,
		RoleAgents:       settings.RoleAgents,
		RoleModels:       settings.RoleModels,
		Agents:           settings.Agents,
		AgentEmailDomain: settings.AgentEmailDomain,
	}

	// Ensure maps are not nil for JSON encoding
	if resp.RoleAgents == nil {
		resp.RoleAgents = make(map[string]string)
	}
	if resp.RoleModels == nil {
		resp.RoleModels = make(map[string]*config.RoleModelConfig)
	}
	if resp.Agents == nil {
		resp.Agents = make(map[string]*config.RuntimeConfig)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding config response: %v", err)
	}
}

// handleAPIConfigPost updates the town settings.
func (h *GUIHandler) handleAPIConfigPost(w http.ResponseWriter, r *http.Request) {
	gtRoot := os.Getenv("GT_ROOT")
	if gtRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "Cannot determine GT_ROOT", http.StatusInternalServerError)
			return
		}
		gtRoot = filepath.Join(home, "gt")
	}

	var req ConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate role names
	validRoles := map[string]bool{
		"mayor": true, "deacon": true, "witness": true,
		"refinery": true, "polecat": true, "crew": true,
	}
	for role := range req.RoleAgents {
		if !validRoles[role] {
			http.Error(w, "Invalid role: "+role, http.StatusBadRequest)
			return
		}
	}
	// Validate role names in RoleModels
	for role := range req.RoleModels {
		if !validRoles[role] {
			http.Error(w, "Invalid role: "+role, http.StatusBadRequest)
			return
		}
	}

	// Validate custom agents have required fields
	for name, agent := range req.Agents {
		if agent == nil {
			http.Error(w, "Agent "+name+" is null", http.StatusBadRequest)
			return
		}
		if agent.Command == "" {
			http.Error(w, "Agent "+name+" missing command", http.StatusBadRequest)
			return
		}
	}

	// Load existing settings to preserve other fields
	settingsPath := config.TownSettingsPath(gtRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		log.Printf("Error loading town settings for update: %v", err)
		http.Error(w, "Failed to load config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update fields from request
	settings.DefaultAgent = req.DefaultAgent
	if settings.DefaultAgent == "" {
		settings.DefaultAgent = "claude"
	}

	settings.RoleAgents = req.RoleAgents
	if settings.RoleAgents == nil {
		settings.RoleAgents = make(map[string]string)
	}
	settings.RoleModels = req.RoleModels
	if settings.RoleModels == nil {
		settings.RoleModels = make(map[string]*config.RoleModelConfig)
	}

	settings.Agents = req.Agents
	if settings.Agents == nil {
		settings.Agents = make(map[string]*config.RuntimeConfig)
	}

	settings.AgentEmailDomain = req.AgentEmailDomain

	// Save settings
	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		log.Printf("Error saving town settings: %v", err)
		http.Error(w, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Town settings updated via WebUI")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "ok"}`))
}
