package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
)

var accountHandlePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// AccountPageData is the data passed to the accounts template.
type AccountPageData struct {
	Title      string
	ActivePage string
}

// handleAccount serves the accounts page.
func (h *GUIHandler) handleAccount(w http.ResponseWriter, r *http.Request) {
	data := AccountPageData{
		Title:      "Accounts",
		ActivePage: "account",
	}
	h.renderTemplate(w, "account.html", data)
}

type accountListItem struct {
	Handle      string `json:"handle"`
	Email       string `json:"email"`
	Description string `json:"description,omitempty"`
	ConfigDir   string `json:"config_dir"`
	IsDefault   bool   `json:"is_default"`
	IsCurrent   bool   `json:"is_current"`
	LoggedIn    bool   `json:"logged_in"`
}

type accountsResponse struct {
	Accounts         []accountListItem `json:"accounts"`
	Default          string            `json:"default,omitempty"`
	CurrentHandle    string            `json:"current_handle,omitempty"`
	CurrentConfigDir string            `json:"current_config_dir,omitempty"`
	CurrentSource    string            `json:"current_source,omitempty"`
	EnvAccount       string            `json:"env_account,omitempty"`
	ClaudeDir        string            `json:"claude_dir,omitempty"`
	ClaudeTarget     string            `json:"claude_target,omitempty"`
	ClaudeIsSymlink  bool              `json:"claude_is_symlink,omitempty"`
	ClaudeExists     bool              `json:"claude_exists,omitempty"`
}

type accountAddRequest struct {
	Handle      string `json:"handle"`
	Email       string `json:"email,omitempty"`
	Description string `json:"description,omitempty"`
}

type accountHandleRequest struct {
	Handle string `json:"handle"`
}

type accountLoginSessionResponse struct {
	SessionID string `json:"session_id"`
}

// handleAPIAccounts returns the list of configured accounts and current status.
func (h *GUIHandler) handleAPIAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := loadAccountsResponse()
	if err != nil {
		http.Error(w, "Failed to load accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPIAccountsAdd registers a new account.
func (h *GUIHandler) handleAPIAccountsAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req accountAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	handle := strings.TrimSpace(req.Handle)
	if !accountHandlePattern.MatchString(handle) {
		http.Error(w, "Invalid handle. Use letters, numbers, dash, or underscore.", http.StatusBadRequest)
		return
	}

	townRoot := webTownRoot()
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := loadAccountsConfig(accountsPath)
	if err != nil {
		http.Error(w, "Failed to load accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, exists := cfg.Accounts[handle]; exists {
		http.Error(w, "Account already exists", http.StatusBadRequest)
		return
	}

	configDir := filepath.Join(config.DefaultAccountsConfigDir(), handle)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		http.Error(w, "Failed to create config directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cfg.Accounts[handle] = config.Account{
		Email:       strings.TrimSpace(req.Email),
		Description: strings.TrimSpace(req.Description),
		ConfigDir:   configDir,
	}

	if cfg.Default == "" {
		cfg.Default = handle
	}

	if err := config.SaveAccountsConfig(accountsPath, cfg); err != nil {
		http.Error(w, "Failed to save accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := loadAccountsResponse()
	if err != nil {
		http.Error(w, "Failed to load accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPIAccountsDefault sets the default account.
func (h *GUIHandler) handleAPIAccountsDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req accountHandleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	handle := strings.TrimSpace(req.Handle)
	if handle == "" {
		http.Error(w, "Handle required", http.StatusBadRequest)
		return
	}

	resp, err := setDefaultAccount(handle)
	if err != nil {
		http.Error(w, "Failed to set default: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPIAccountsSwitch switches the ~/.claude symlink to a target account.
func (h *GUIHandler) handleAPIAccountsSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req accountHandleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	targetHandle := strings.TrimSpace(req.Handle)
	if targetHandle == "" {
		http.Error(w, "Handle required", http.StatusBadRequest)
		return
	}

	resp, err := setDefaultAccount(targetHandle)
	if err != nil {
		http.Error(w, "Failed to switch account: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func loadAccountsResponse() (accountsResponse, error) {
	townRoot := webTownRoot()
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := loadAccountsConfig(accountsPath)
	if err != nil {
		return accountsResponse{}, err
	}

	configDir, handle, err := config.ResolveAccountConfigDir(accountsPath, "")
	if err != nil {
		return accountsResponse{}, err
	}

	resp := accountsResponse{
		Default:          cfg.Default,
		CurrentHandle:    handle,
		CurrentConfigDir: configDir,
		EnvAccount:       os.Getenv("GT_ACCOUNT"),
	}

	if resp.EnvAccount != "" {
		resp.CurrentSource = "env"
	} else if handle != "" {
		resp.CurrentSource = "default"
	} else {
		resp.CurrentSource = "none"
	}

	resp.Accounts = buildAccountList(cfg, handle)
	populateClaudeStatus(&resp)

	return resp, nil
}

func setDefaultAccount(handle string) (accountsResponse, error) {
	townRoot := webTownRoot()
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := loadAccountsConfig(accountsPath)
	if err != nil {
		return accountsResponse{}, err
	}

	if _, exists := cfg.Accounts[handle]; !exists {
		return accountsResponse{}, errors.New("account not found")
	}

	cfg.Default = handle
	if err := config.SaveAccountsConfig(accountsPath, cfg); err != nil {
		return accountsResponse{}, err
	}

	return loadAccountsResponse()
}

func loadAccountsConfig(path string) (*config.AccountsConfig, error) {
	cfg, err := config.LoadAccountsConfig(path)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return config.NewAccountsConfig(), nil
		}
		return nil, err
	}
	return cfg, nil
}

func buildAccountList(cfg *config.AccountsConfig, currentHandle string) []accountListItem {
	if cfg == nil || len(cfg.Accounts) == 0 {
		return nil
	}

	handles := make([]string, 0, len(cfg.Accounts))
	for handle := range cfg.Accounts {
		handles = append(handles, handle)
	}
	sort.Strings(handles)

	items := make([]accountListItem, 0, len(handles))
	for _, handle := range handles {
		acct := cfg.Accounts[handle]
		items = append(items, accountListItem{
			Handle:      handle,
			Email:       acct.Email,
			Description: acct.Description,
			ConfigDir:   acct.ConfigDir,
			IsDefault:   handle == cfg.Default,
			IsCurrent:   handle == currentHandle,
			LoggedIn:    accountHasCredentials(acct.ConfigDir),
		})
	}

	return items
}

func populateClaudeStatus(resp *accountsResponse) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	claudeDir := filepath.Join(home, ".claude")
	fileInfo, err := os.Lstat(claudeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		return
	}

	resp.ClaudeDir = claudeDir
	resp.ClaudeExists = true
	resp.ClaudeIsSymlink = fileInfo.Mode()&os.ModeSymlink != 0
	if resp.ClaudeIsSymlink {
		linkTarget, err := os.Readlink(claudeDir)
		if err == nil {
			resp.ClaudeTarget = linkTarget
		}
	}
}

func accountHasCredentials(configDir string) bool {
	if strings.TrimSpace(configDir) == "" {
		return false
	}
	credPath := filepath.Join(configDir, ".credentials.json")
	info, err := os.Stat(credPath)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func accountLoginSessionName(handle string) string {
	return "gt-login-" + handle
}

func (h *GUIHandler) handleAPIAccountsLoginStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req accountHandleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	handle := strings.TrimSpace(req.Handle)
	if !accountHandlePattern.MatchString(handle) {
		http.Error(w, "Invalid handle", http.StatusBadRequest)
		return
	}

	townRoot := webTownRoot()
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := loadAccountsConfig(accountsPath)
	if err != nil {
		http.Error(w, "Failed to load accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	acct := cfg.GetAccount(handle)
	if acct == nil {
		http.Error(w, "Account not found", http.StatusBadRequest)
		return
	}

	configDir := strings.TrimSpace(acct.ConfigDir)
	if configDir == "" {
		http.Error(w, "Account missing config dir", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		http.Error(w, "Failed to create config dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID := accountLoginSessionName(handle)
	if err := ensureTmuxSession(sessionID, configDir); err != nil {
		http.Error(w, "Failed to start login session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(accountLoginSessionResponse{SessionID: sessionID})
}

func (h *GUIHandler) handleAPIAccountsLoginStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req accountHandleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	handle := strings.TrimSpace(req.Handle)
	if !accountHandlePattern.MatchString(handle) {
		http.Error(w, "Invalid handle", http.StatusBadRequest)
		return
	}

	sessionID := accountLoginSessionName(handle)
	cmd, cancel := command("tmux", "kill-session", "-t", sessionID)
	defer cancel()
	_ = cmd.Run()

	w.WriteHeader(http.StatusNoContent)
}

func ensureTmuxSession(sessionID, configDir string) error {
	if tmuxSessionExists(sessionID) {
		return nil
	}

	cmd, cancel := command("tmux", "new-session", "-d", "-s", sessionID, "-c", configDir, "env", "CLAUDE_CONFIG_DIR="+configDir, "claude", "--dangerously-skip-permissions")
	defer cancel()
	return cmd.Run()
}

func tmuxSessionExists(sessionID string) bool {
	cmd, cancel := command("tmux", "has-session", "-t", sessionID)
	defer cancel()
	return cmd.Run() == nil
}
