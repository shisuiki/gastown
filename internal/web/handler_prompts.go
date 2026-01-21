package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

// PromptResponse represents the API response for a prompt.
type PromptResponse struct {
	Role         string `json:"role"`
	Rig          string `json:"rig,omitempty"`
	Source       string `json:"source"` // "inline", "file", "builtin"
	Content      string `json:"content"`
	FilePath     string `json:"file_path,omitempty"`
	ResolvedFrom string `json:"resolved_from,omitempty"` // "rig", "town", "builtin"
}

// PromptRequest represents the API request for updating a prompt.
type PromptRequest struct {
	Content  string `json:"content"`
	Source   string `json:"source,omitempty"`    // "inline" or "file"
	FilePath string `json:"file_path,omitempty"` // required if source="file"
}

// PromptTemplateFile represents a prompt template file.
type PromptTemplateFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// PromptTemplatesResponse represents the API response for template files.
type PromptTemplatesResponse struct {
	Kind  string               `json:"kind"`
	Base  string               `json:"base"`
	Files []PromptTemplateFile `json:"files"`
}

// PromptTemplateUpdateRequest represents the API request for updating a template file.
type PromptTemplateUpdateRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ClaudeFileResponse represents the API response for CLAUDE.md.
type ClaudeFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

// ClaudeFileRequest represents the API request for updating CLAUDE.md.
type ClaudeFileRequest struct {
	Content string `json:"content"`
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

// handleAPIPromptTemplates handles GET and POST for /api/prompts/templates.
func (h *GUIHandler) handleAPIPromptTemplates(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		http.Error(w, "Template kind required", http.StatusBadRequest)
		return
	}

	repoRoot, err := promptsRepoRoot()
	if err != nil {
		http.Error(w, "Failed to locate repo root", http.StatusInternalServerError)
		return
	}

	dir, base, allowedExts, err := promptTemplateConfig(kind, repoRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		files, err := readPromptTemplates(dir, allowedExts)
		if err != nil {
			log.Printf("Error reading prompt templates (%s): %v", kind, err)
			http.Error(w, "Failed to read prompt templates", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PromptTemplatesResponse{
			Kind:  kind,
			Base:  base,
			Files: files,
		})
	case http.MethodPost:
		var req PromptTemplateUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := writePromptTemplate(dir, allowedExts, &req); err != nil {
			log.Printf("Error updating prompt template (%s): %v", kind, err)
			http.Error(w, "Failed to update prompt template: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok"}`))
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIPromptClaude handles GET and POST for /api/prompts/claude.
func (h *GUIHandler) handleAPIPromptClaude(w http.ResponseWriter, r *http.Request) {
	gtRoot := getGTRoot()
	claudePath := filepath.Join(gtRoot, "mayor", "CLAUDE.md")

	switch r.Method {
	case http.MethodGet:
		content := []byte{}
		exists := false
		if info, err := os.Stat(claudePath); err == nil && !info.IsDir() {
			exists = true
			content, err = os.ReadFile(claudePath)
			if err != nil {
				log.Printf("Error reading CLAUDE.md: %v", err)
				http.Error(w, "Failed to read CLAUDE.md", http.StatusInternalServerError)
				return
			}
		} else if err != nil && !os.IsNotExist(err) {
			log.Printf("Error checking CLAUDE.md: %v", err)
			http.Error(w, "Failed to read CLAUDE.md", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ClaudeFileResponse{
			Path:    filepath.ToSlash(filepath.Join("mayor", "CLAUDE.md")),
			Content: string(content),
			Exists:  exists,
		})
	case http.MethodPost:
		var req ClaudeFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(filepath.Dir(claudePath), 0755); err != nil {
			log.Printf("Error creating CLAUDE.md directory: %v", err)
			http.Error(w, "Failed to update CLAUDE.md", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(claudePath, []byte(req.Content), 0644); err != nil {
			log.Printf("Error writing CLAUDE.md: %v", err)
			http.Error(w, "Failed to update CLAUDE.md", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok"}`))
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
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
	cmd, cancel := command("git", "add", ".")
	defer cancel()
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %v, output: %s", err, output)
	}

	// Commit
	commitMsg := fmt.Sprintf("Update prompt for role %s via WebUI", role)
	cmd, cancel = command("git", "commit", "-m", commitMsg)
	defer cancel()
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		// If there are no changes, git commit returns error, that's okay
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %v, output: %s", err, output)
	}

	// Push
	cmd, cancel = command("git", "push", "origin", "HEAD")
	defer cancel()
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %v, output: %s", err, output)
	}

	return nil
}

func promptTemplateConfig(kind, repoRoot string) (string, string, map[string]bool, error) {
	switch kind {
	case "system":
		return filepath.Join(repoRoot, "internal", "templates", "system-prompts"),
			filepath.ToSlash(filepath.Join("internal", "templates", "system-prompts")),
			map[string]bool{".md": true},
			nil
	case "roles":
		return filepath.Join(repoRoot, "internal", "templates", "roles"),
			filepath.ToSlash(filepath.Join("internal", "templates", "roles")),
			map[string]bool{".tmpl": true},
			nil
	default:
		return "", "", nil, fmt.Errorf("invalid template kind: %s", kind)
	}
}

func readPromptTemplates(dir string, allowedExts map[string]bool) ([]PromptTemplateFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	files := make([]PromptTemplateFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !allowedExts[ext] {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		files = append(files, PromptTemplateFile{
			Name:    name,
			Path:    name,
			Content: string(content),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return files, nil
}

func writePromptTemplate(dir string, allowedExts map[string]bool, req *PromptTemplateUpdateRequest) error {
	if req.Path == "" {
		return fmt.Errorf("missing path")
	}
	fullPath, err := safeTemplatePath(dir, req.Path, allowedExts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("creating template directory: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		return fmt.Errorf("writing template file: %w", err)
	}
	return nil
}

func safeTemplatePath(baseDir, relPath string, allowedExts map[string]bool) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("missing path")
	}
	if strings.Contains(relPath, "\x00") {
		return "", fmt.Errorf("invalid path")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("absolute paths not allowed")
	}

	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid path")
	}

	fullPath := filepath.Join(baseDir, clean)
	baseDir = filepath.Clean(baseDir)
	if fullPath != baseDir && !strings.HasPrefix(fullPath, baseDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes template root")
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	if !allowedExts[ext] {
		return "", fmt.Errorf("unsupported file extension")
	}

	return fullPath, nil
}

func promptsRepoRoot() (string, error) {
	for _, key := range []string{"GASTOWN_SRC", "GT_ROOT"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		if root, err := findRepoRoot(value); err == nil {
			return root, nil
		}
	}

	_, cwd, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return "", err
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	return findRepoRoot(cwd)
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
