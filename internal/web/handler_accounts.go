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

	townRoot := webTownRoot()
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := loadAccountsConfig(accountsPath)
	if err != nil {
		http.Error(w, "Failed to load accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, exists := cfg.Accounts[handle]; !exists {
		http.Error(w, "Account not found", http.StatusBadRequest)
		return
	}

	cfg.Default = handle
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

	townRoot := webTownRoot()
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := loadAccountsConfig(accountsPath)
	if err != nil {
		http.Error(w, "Failed to load accounts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	targetAcct := cfg.GetAccount(targetHandle)
	if targetAcct == nil {
		http.Error(w, "Account not found", http.StatusBadRequest)
		return
	}

	if err := switchClaudeAccount(cfg, targetHandle); err != nil {
		http.Error(w, "Failed to switch account: "+err.Error(), http.StatusInternalServerError)
		return
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

func switchClaudeAccount(cfg *config.AccountsConfig, targetHandle string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	claudeDir := filepath.Join(home, ".claude")

	targetAcct := cfg.GetAccount(targetHandle)
	if targetAcct == nil {
		return errors.New("account not found")
	}

	fileInfo, err := os.Lstat(claudeDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var currentHandle string
	if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(claudeDir)
		if err != nil {
			return err
		}
		for handle, acct := range cfg.Accounts {
			if acct.ConfigDir == linkTarget {
				currentHandle = handle
				break
			}
		}
	}

	if currentHandle == targetHandle {
		cfg.Default = targetHandle
		return nil
	}

	if err == nil && fileInfo.Mode()&os.ModeSymlink == 0 && fileInfo.IsDir() {
		if currentHandle == "" && cfg.Default != "" {
			currentHandle = cfg.Default
		}

		if currentHandle == "" {
			return errors.New("~/.claude exists but no default account is set")
		}

		currentAcct := cfg.GetAccount(currentHandle)
		if currentAcct == nil {
			return errors.New("current account not found")
		}

		if _, err := os.Stat(currentAcct.ConfigDir); err == nil {
			if err := os.RemoveAll(currentAcct.ConfigDir); err != nil {
				return err
			}
		}

		if err := os.Rename(claudeDir, currentAcct.ConfigDir); err != nil {
			return err
		}
	} else if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(claudeDir); err != nil {
			return err
		}
	}

	if err := os.Symlink(targetAcct.ConfigDir, claudeDir); err != nil {
		return err
	}

	cfg.Default = targetHandle
	return nil
}
