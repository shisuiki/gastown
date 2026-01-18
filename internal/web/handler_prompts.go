package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// PromptResponse represents the API response for a prompt.
type PromptResponse struct {
	Role       string `json:"role"`
	Rig        string `json:"rig,omitempty"`
	Source     string `json:"source"` // "inline", "file", "builtin"
	Content    string `json:"content"`
	FilePath   string `json:"file_path,omitempty"`
	ResolvedFrom string `json:"resolved_from,omitempty"` // "rig", "town", "builtin"
}

// PromptRequest represents the API request for updating a prompt.
type PromptRequest struct {
	Content  string `json:"content"`
	Source   string `json:"source,omitempty"` // "inline" or "file"
	FilePath string `json:"file_path,omitempty"` // required if source="file"
}

// handleAPIPrompts handles GET and POST for /api/prompts/{role}.
func (h *GUIHandler) handleAPIPrompts(w http.ResponseWriter, r *http.Request) {
	// Extract role from path: /api/prompts/{role}
	pathPrefix := "/api/prompts/"
	if !strings.HasPrefix(r.URL.Path, pathPrefix) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	role := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if role == "" {
		http.Error(w, "Role required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleAPIPromptsGet(w, r, role)
	case http.MethodPost:
		h.handleAPIPromptsPost(w, r, role)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIPromptsGet returns the current prompt for a role.
func (h *GUIHandler) handleAPIPromptsGet(w http.ResponseWriter, r *http.Request, role string) {
	rig := r.URL.Query().Get("rig")
	gtRoot := getGTRoot()

	// Determine if we're looking at rig-level or town-level
	var townRoot, rigPath string
	townRoot = gtRoot
	if rig != "" {
		rigPath = filepath.Join(gtRoot, rig)
	}

	// Use config.ResolveSystemPrompt to get the resolved content
	content, err := config.ResolveSystemPrompt(role, townRoot, rigPath)
	if err != nil {
		log.Printf("Error resolving system prompt for role %s: %v", role, err)
		http.Error(w, "Failed to resolve prompt: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine source and resolved from
	source := "builtin"
	resolvedFrom := "builtin"
	filePath := ""

	// Check rig-level override
	if rigPath != "" {
		rigSettings, err := config.LoadRigSettings(config.RigSettingsPath(rigPath))
		if err == nil && rigSettings.SystemPrompts != nil {
			if prompt, ok := rigSettings.SystemPrompts[role]; ok && prompt != "" {
				resolvedFrom = "rig"
				if strings.HasPrefix(prompt, "file:") {
					source = "file"
					filePath = strings.TrimPrefix(prompt, "file:")
				} else {
					source = "inline"
				}
			}
		}
	}

	// If not rig-level, check town-level
	if resolvedFrom == "builtin" {
		townSettings, err := config.LoadOrCreateTownSettings(config.TownSettingsPath(townRoot))
		if err == nil && townSettings.SystemPrompts != nil {
			if prompt, ok := townSettings.SystemPrompts[role]; ok && prompt != "" {
				resolvedFrom = "town"
				if strings.HasPrefix(prompt, "file:") {
					source = "file"
					filePath = strings.TrimPrefix(prompt, "file:")
				} else {
					source = "inline"
				}
			}
		}
	}

	resp := PromptResponse{
		Role:         role,
		Rig:          rig,
		Source:       source,
		Content:      content,
		FilePath:     filePath,
		ResolvedFrom: resolvedFrom,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding prompt response: %v", err)
	}
}

// handleAPIPromptsPost updates the prompt for a role.
func (h *GUIHandler) handleAPIPromptsPost(w http.ResponseWriter, r *http.Request, role string) {
	rig := r.URL.Query().Get("rig")
	gtRoot := getGTRoot()

	// Parse request
	var req PromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate role
	validRoles := map[string]bool{
		"mayor": true, "deacon": true, "witness": true,
		"refinery": true, "polecat": true, "crew": true,
	}
	if !validRoles[role] {
		http.Error(w, "Invalid role: "+role, http.StatusBadRequest)
		return
	}

	// Determine target (town or rig)
	var targetPath string
	var isRig bool
	if rig != "" {
		targetPath = filepath.Join(gtRoot, rig)
		isRig = true
	} else {
		targetPath = gtRoot
		isRig = false
	}

	// Update prompt
	if err := updatePrompt(targetPath, isRig, role, &req); err != nil {
		log.Printf("Error updating prompt for role %s: %v", role, err)
		http.Error(w, "Failed to update prompt: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Commit and push changes
	if err := gitCommitAndPush(targetPath, role); err != nil {
		log.Printf("Error committing prompt changes: %v", err)
		// Don't fail the request - just log
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "ok"}`))
}

// updatePrompt updates the prompt for a role in either town or rig settings.
func updatePrompt(targetPath string, isRig bool, role string, req *PromptRequest) error {
	// Determine prompt value based on source
	source := req.Source
	if source == "" {
		source = "inline"
	}

	var promptValue string
	if source == "file" {
		// Determine file path
		filePath := req.FilePath
		if filePath == "" {
			// Generate default file path
			var promptsDir string
			if isRig {
				promptsDir = filepath.Join(targetPath, "prompts")
			} else {
				promptsDir = filepath.Join(targetPath, "settings", "prompts")
			}
			if err := os.MkdirAll(promptsDir, 0755); err != nil {
				return fmt.Errorf("creating prompts directory: %w", err)
			}
			filePath = filepath.Join(promptsDir, role+".md")
		} else {
			// Ensure file path is absolute or relative to targetPath
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(targetPath, filePath)
			}
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				return fmt.Errorf("creating directory for prompt file: %w", err)
			}
		}

		// Write prompt file
		if err := os.WriteFile(filePath, []byte(req.Content), 0644); err != nil {
			return fmt.Errorf("writing prompt file: %w", err)
		}
		promptValue = "file:" + filePath
	} else {
		// Inline prompt
		promptValue = req.Content
	}

	// Update settings
	if isRig {
		return updateRigPrompt(targetPath, role, promptValue)
	} else {
		return updateTownPrompt(targetPath, role, promptValue)
	}
}

// updateRigPrompt updates the system prompt in rig settings.
func updateRigPrompt(rigPath, role, promptValue string) error {
	settingsPath := config.RigSettingsPath(rigPath)
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		// If file doesn't exist, create empty settings
		settings = config.NewRigSettings()
	}

	if settings.SystemPrompts == nil {
		settings.SystemPrompts = make(map[string]string)
	}
	settings.SystemPrompts[role] = promptValue

	return config.SaveRigSettings(settingsPath, settings)
}

// updateTownPrompt updates the system prompt in town settings.
func updateTownPrompt(townRoot, role, promptValue string) error {
	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if settings.SystemPrompts == nil {
		settings.SystemPrompts = make(map[string]string)
	}
	settings.SystemPrompts[role] = promptValue

	return config.SaveTownSettings(settingsPath, settings)
}

// gitCommitAndPush commits and pushes prompt changes to the git remote.
func gitCommitAndPush(repoPath, role string) error {
	// Check if the directory is a git repository
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Not a git repo, skip git operations
		return nil
	}

	// Determine which file(s) to add based on repo type
	// For simplicity, add all changes in the repo
	// We'll run git add . in the repo directory
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %v, output: %s", err, output)
	}

	// Commit
	commitMsg := fmt.Sprintf("Update prompt for role %s via WebUI", role)
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		// If there are no changes, git commit returns error, that's okay
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %v, output: %s", err, output)
	}

	// Push
	cmd = exec.Command("git", "push", "origin", "HEAD")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %v, output: %s", err, output)
	}

	return nil
}

// PromptsPageData is the data passed to the prompts template.
type PromptsPageData struct {
	Title      string
	ActivePage string
}

// handlePrompts serves the prompts page.
func (h *GUIHandler) handlePrompts(w http.ResponseWriter, r *http.Request) {
	data := PromptsPageData{
		Title:      "System Prompts",
		ActivePage: "prompts",
	}
	h.renderTemplate(w, "prompts.html", data)
}