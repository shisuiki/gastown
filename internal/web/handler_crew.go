package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/townlog"
	"github.com/steveyegge/gastown/internal/workspace"
)

// CrewPageData is the data passed to the crew template.
type CrewPageData struct {
	Title      string
	ActivePage string
}

// CrewStatusItem represents detailed status for a crew worker.
type CrewStatusItem struct {
	Name         string   `json:"name"`
	Rig          string   `json:"rig"`
	Path         string   `json:"path"`
	Branch       string   `json:"branch"`
	HasSession   bool     `json:"has_session"`
	SessionID    string   `json:"session_id,omitempty"`
	GitClean     bool     `json:"git_clean"`
	GitModified  []string `json:"git_modified"`
	GitUntracked []string `json:"git_untracked"`
	MailTotal    int      `json:"mail_total"`
	MailUnread   int      `json:"mail_unread"`
}

// CrewListResponse contains crew list data.
type CrewListResponse struct {
	Crew   []CrewStatusItem `json:"crew"`
	Count  int              `json:"count"`
	Errors []string         `json:"errors,omitempty"`
}

// CrewActionRequest represents crew action requests.
type CrewActionRequest struct {
	Action        string   `json:"action"`
	Rig           string   `json:"rig,omitempty"`
	Name          string   `json:"name,omitempty"`
	Names         []string `json:"names,omitempty"`
	NewName       string   `json:"new_name,omitempty"`
	Force         bool     `json:"force,omitempty"`
	Purge         bool     `json:"purge,omitempty"`
	All           bool     `json:"all,omitempty"`
	Message       string   `json:"message,omitempty"`
	Account       string   `json:"account,omitempty"`
	AgentOverride string   `json:"agent_override,omitempty"`
	Branch        bool     `json:"branch,omitempty"`
}

// CrewActionResponse represents the result of a crew action.
type CrewActionResponse struct {
	Success  bool     `json:"success"`
	Output   string   `json:"output,omitempty"`
	Error    string   `json:"error,omitempty"`
	Sessions []string `json:"sessions,omitempty"`
}

// handleCrew serves the crew page.
func (h *GUIHandler) handleCrew(w http.ResponseWriter, r *http.Request) {
	data := CrewPageData{
		Title:      "Crew",
		ActivePage: "crew",
	}
	h.renderTemplate(w, "crew.html", data)
}

// handleAPICrewList returns crew list and status data.
func (h *GUIHandler) handleAPICrewList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rigFilter := strings.TrimSpace(r.URL.Query().Get("rig"))
	if strings.EqualFold(rigFilter, "all") {
		rigFilter = ""
	}

	cacheKey := "crew_list_all"
	if rigFilter != "" {
		cacheKey = "crew_list_" + sanitizeKey(rigFilter)
	}

	cached := h.cache.GetStaleOrRefresh(cacheKey, CrewCacheTTL, func() interface{} {
		return h.fetchCrewList(rigFilter)
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	result := h.fetchCrewList(rigFilter)
	h.cache.Set(cacheKey, result, CrewCacheTTL)
	json.NewEncoder(w).Encode(result)
}

// handleAPICrewAction handles crew lifecycle actions.
func (h *GUIHandler) handleAPICrewAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(CrewActionResponse{
			Success: false,
			Error:   "method not allowed",
		})
		return
	}

	var req CrewActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(CrewActionResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	if req.Action == "" {
		json.NewEncoder(w).Encode(CrewActionResponse{
			Success: false,
			Error:   "action is required",
		})
		return
	}

	resp, err := h.runCrewAction(req)
	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
	}

	if resp.Success {
		h.cache.InvalidatePrefix("crew_list_")
		h.cache.Invalidate("crew_list_all")
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *GUIHandler) fetchCrewList(rigFilter string) CrewListResponse {
	rigs, err := h.discoverRigs()
	if err != nil {
		return CrewListResponse{
			Crew:   []CrewStatusItem{},
			Count:  0,
			Errors: []string{err.Error()},
		}
	}

	if rigFilter != "" {
		var filtered []*rig.Rig
		for _, r := range rigs {
			if r.Name == rigFilter {
				filtered = append(filtered, r)
				break
			}
		}
		if len(filtered) == 0 {
			return CrewListResponse{
				Crew:   []CrewStatusItem{},
				Count:  0,
				Errors: []string{fmt.Sprintf("rig %q not found", rigFilter)},
			}
		}
		rigs = filtered
	}

	t := tmux.NewTmux()
	var items []CrewStatusItem
	var errorsList []string

	for _, r := range rigs {
		crewMgr := crew.NewManager(r, git.NewGit(r.Path))
		workers, err := crewMgr.List()
		if err != nil {
			errorsList = append(errorsList, fmt.Sprintf("listing crew in %s: %v", r.Name, err))
			continue
		}

		for _, w := range workers {
			sessionID := crewMgr.SessionName(w.Name)
			hasSession, _ := t.HasSession(sessionID)

			crewGit := git.NewGit(w.ClonePath)
			gitStatus, _ := crewGit.Status()
			branch, _ := crewGit.CurrentBranch()

			gitClean := true
			modified := []string{}
			untracked := []string{}
			if gitStatus != nil {
				gitClean = gitStatus.Clean
				modified = append(modified, gitStatus.Modified...)
				modified = append(modified, gitStatus.Added...)
				modified = append(modified, gitStatus.Deleted...)
				untracked = gitStatus.Untracked
			}

			mailTotal := 0
			mailUnread := 0
			mailDir := filepath.Join(w.ClonePath, "mail")
			if _, err := os.Stat(mailDir); err == nil {
				mailbox := mail.NewMailbox(mailDir)
				mailTotal, mailUnread, _ = mailbox.Count()
			}

			item := CrewStatusItem{
				Name:         w.Name,
				Rig:          r.Name,
				Path:         w.ClonePath,
				Branch:       branch,
				HasSession:   hasSession,
				GitClean:     gitClean,
				GitModified:  modified,
				GitUntracked: untracked,
				MailTotal:    mailTotal,
				MailUnread:   mailUnread,
			}
			if hasSession {
				item.SessionID = sessionID
			}

			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Rig == items[j].Rig {
			return items[i].Name < items[j].Name
		}
		return items[i].Rig < items[j].Rig
	})

	return CrewListResponse{
		Crew:   items,
		Count:  len(items),
		Errors: errorsList,
	}
}

func (h *GUIHandler) runCrewAction(req CrewActionRequest) (CrewActionResponse, error) {
	switch req.Action {
	case "add":
		return h.runCrewAdd(req)
	case "remove":
		return h.runCrewRemove(req)
	case "rename":
		return h.runCrewRename(req)
	case "start":
		return h.runCrewStart(req)
	case "stop":
		return h.runCrewStop(req)
	case "restart":
		return h.runCrewRestart(req)
	case "refresh":
		return h.runCrewRefresh(req)
	case "pristine":
		return h.runCrewPristine(req)
	default:
		return CrewActionResponse{Success: false}, fmt.Errorf("unknown action: %s", req.Action)
	}
}

func (h *GUIHandler) runCrewAdd(req CrewActionRequest) (CrewActionResponse, error) {
	if req.Rig == "" {
		return CrewActionResponse{}, errors.New("rig is required")
	}
	names := crewNamesFromRequest(req)
	if len(names) == 0 {
		return CrewActionResponse{}, errors.New("crew name is required")
	}

	townRoot, r, err := h.resolveRig(req.Rig)
	if err != nil {
		return CrewActionResponse{}, err
	}

	crewMgr := crew.NewManager(r, git.NewGit(r.Path))
	bd := beads.New(beads.ResolveBeadsDir(r.Path))

	var output []string
	var sessions []string
	var failed []string

	for _, name := range names {
		worker, err := crewMgr.Add(name, req.Branch)
		if err != nil {
			if errors.Is(err, crew.ErrCrewExists) {
				failed = append(failed, fmt.Sprintf("%s (exists)", name))
				continue
			}
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			continue
		}

		output = append(output, fmt.Sprintf("Created crew workspace: %s/%s", r.Name, name))
		output = append(output, fmt.Sprintf("Path: %s", worker.ClonePath))
		output = append(output, fmt.Sprintf("Branch: %s", worker.Branch))

		prefix := beads.GetPrefixForRig(townRoot, r.Name)
		crewID := beads.CrewBeadIDWithPrefix(prefix, r.Name, name)
		if _, err := bd.Show(crewID); err != nil {
			fields := &beads.AgentFields{
				RoleType:   "crew",
				Rig:        r.Name,
				AgentState: "idle",
				RoleBead:   beads.RoleBeadIDTown("crew"),
			}
			desc := fmt.Sprintf("Crew worker %s in %s - human-managed persistent workspace.", name, r.Name)
			if _, err := bd.CreateAgentBead(crewID, desc, fields); err == nil {
				output = append(output, fmt.Sprintf("Agent bead: %s", crewID))
			}
		}

		sessions = append(sessions, crewMgr.SessionName(name))
	}

	if len(failed) > 0 {
		output = append(output, fmt.Sprintf("Failed: %s", strings.Join(failed, ", ")))
	}

	return CrewActionResponse{
		Success:  len(failed) < len(names),
		Output:   strings.Join(output, "\n"),
		Sessions: sessions,
	}, nil
}

func (h *GUIHandler) runCrewRemove(req CrewActionRequest) (CrewActionResponse, error) {
	names := crewNamesFromRequest(req)
	if len(names) == 0 {
		return CrewActionResponse{}, errors.New("crew name is required")
	}
	if req.Rig == "" {
		return CrewActionResponse{}, errors.New("rig is required")
	}

	townRoot, r, err := h.resolveRig(req.Rig)
	if err != nil {
		return CrewActionResponse{}, err
	}

	crewMgr := crew.NewManager(r, git.NewGit(r.Path))
	t := tmux.NewTmux()

	var output []string
	var failed []string

	for _, name := range names {
		sessionID := crewMgr.SessionName(name)
		if !req.Force {
			if hasSession, _ := t.HasSession(sessionID); hasSession {
				failed = append(failed, fmt.Sprintf("%s (session running)", name))
				continue
			}
		}

		if hasSession, _ := t.HasSession(sessionID); hasSession {
			if err := t.KillSessionWithProcesses(sessionID); err != nil {
				failed = append(failed, fmt.Sprintf("%s (kill failed: %v)", name, err))
				continue
			}
			output = append(output, fmt.Sprintf("Killed session %s", sessionID))
		}

		crewPath := filepath.Join(r.Path, "crew", name)
		gitPath := filepath.Join(crewPath, ".git")
		isWorktree := false
		if info, err := os.Stat(gitPath); err == nil && !info.IsDir() {
			isWorktree = true
		}

		if isWorktree {
			mayorRigPath := constants.RigMayorPath(r.Path)
			removeArgs := []string{"worktree", "remove", crewPath}
			if req.Force || req.Purge {
				removeArgs = []string{"worktree", "remove", "--force", crewPath}
			}
			cmd, cancel := command("git", removeArgs...)
			cmd.Dir = mayorRigPath
			out, err := cmd.CombinedOutput()
			cancel()
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s (worktree remove failed: %v)", name, err))
				if len(out) > 0 {
					output = append(output, strings.TrimSpace(string(out)))
				}
				continue
			}
			output = append(output, fmt.Sprintf("Removed crew worktree: %s/%s", r.Name, name))
		} else {
			if err := crewMgr.Remove(name, req.Force || req.Purge); err != nil {
				switch {
				case errors.Is(err, crew.ErrCrewNotFound):
					failed = append(failed, fmt.Sprintf("%s (not found)", name))
				case errors.Is(err, crew.ErrHasChanges):
					failed = append(failed, fmt.Sprintf("%s (uncommitted changes)", name))
				default:
					failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
				}
				continue
			}
			output = append(output, fmt.Sprintf("Removed crew workspace: %s/%s", r.Name, name))
		}

		prefix := beads.GetPrefixForRig(townRoot, r.Name)
		agentBeadID := beads.CrewBeadIDWithPrefix(prefix, r.Name, name)

		if req.Purge {
			deleteArgs := []string{"delete", agentBeadID, "--force"}
			deleteCmd, cancel := command("bd", deleteArgs...)
			deleteCmd.Dir = r.Path
			out, err := deleteCmd.CombinedOutput()
			cancel()
			if err == nil {
				if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
					output = append(output, trimmed)
				}
				output = append(output, fmt.Sprintf("Deleted agent bead: %s", agentBeadID))
			}

			agentAddr := fmt.Sprintf("%s/crew/%s", r.Name, name)
			unassignArgs := []string{"list", "--assignee=" + agentAddr, "--format=id"}
			unassignCmd, cancel := command("bd", unassignArgs...)
			unassignCmd.Dir = r.Path
			out, err = unassignCmd.CombinedOutput()
			cancel()
			if err == nil {
				ids := strings.Fields(strings.TrimSpace(string(out)))
				for _, id := range ids {
					updateCmd, cancel := command("bd", "update", id, "--unassign")
					updateCmd.Dir = r.Path
					if _, err := updateCmd.CombinedOutput(); err == nil {
						output = append(output, fmt.Sprintf("Unassigned: %s", id))
					}
					cancel()
				}
			}
		} else {
			closeArgs := []string{"close", agentBeadID, "--reason=Crew workspace removed"}
			if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
				closeArgs = append(closeArgs, "--session="+sessionID)
			}
			closeCmd, cancel := command("bd", closeArgs...)
			closeCmd.Dir = r.Path
			out, err := closeCmd.CombinedOutput()
			cancel()
			if err == nil {
				if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
					output = append(output, trimmed)
				}
				output = append(output, fmt.Sprintf("Closed agent bead: %s", agentBeadID))
			}
		}
	}

	if len(failed) > 0 {
		output = append(output, fmt.Sprintf("Failed: %s", strings.Join(failed, ", ")))
	}

	return CrewActionResponse{
		Success: len(failed) < len(names),
		Output:  strings.Join(output, "\n"),
	}, nil
}

func (h *GUIHandler) runCrewRename(req CrewActionRequest) (CrewActionResponse, error) {
	if req.Rig == "" {
		return CrewActionResponse{}, errors.New("rig is required")
	}
	if req.Name == "" || req.NewName == "" {
		return CrewActionResponse{}, errors.New("name and new_name are required")
	}

	_, r, err := h.resolveRig(req.Rig)
	if err != nil {
		return CrewActionResponse{}, err
	}

	crewMgr := crew.NewManager(r, git.NewGit(r.Path))
	t := tmux.NewTmux()
	oldSessionID := crewMgr.SessionName(req.Name)
	if hasSession, _ := t.HasSession(oldSessionID); hasSession {
		if err := t.KillSession(oldSessionID); err != nil {
			return CrewActionResponse{}, fmt.Errorf("killing old session: %w", err)
		}
	}

	if err := crewMgr.Rename(req.Name, req.NewName); err != nil {
		switch {
		case errors.Is(err, crew.ErrCrewNotFound):
			return CrewActionResponse{}, fmt.Errorf("crew workspace %q not found", req.Name)
		case errors.Is(err, crew.ErrCrewExists):
			return CrewActionResponse{}, fmt.Errorf("crew workspace %q already exists", req.NewName)
		default:
			return CrewActionResponse{}, fmt.Errorf("renaming crew workspace: %w", err)
		}
	}

	output := fmt.Sprintf("Renamed crew workspace: %s/%s -> %s/%s", r.Name, req.Name, r.Name, req.NewName)
	return CrewActionResponse{
		Success: true,
		Output:  output,
	}, nil
}

func (h *GUIHandler) runCrewStart(req CrewActionRequest) (CrewActionResponse, error) {
	if req.Rig == "" {
		return CrewActionResponse{}, errors.New("rig is required")
	}

	townRoot, r, err := h.resolveRig(req.Rig)
	if err != nil {
		return CrewActionResponse{}, err
	}

	crewMgr := crew.NewManager(r, git.NewGit(r.Path))
	names, err := h.resolveCrewTargets(req, crewMgr)
	if err != nil {
		return CrewActionResponse{}, err
	}

	claudeConfigDir, _, _ := resolveAccountConfigDir(townRoot, req.Account)

	opts := crew.StartOptions{
		Account:         req.Account,
		ClaudeConfigDir: claudeConfigDir,
		AgentOverride:   req.AgentOverride,
	}

	var output []string
	var sessions []string
	var failed []string
	var skipped []string

	for _, name := range names {
		err := crewMgr.Start(name, opts)
		if errors.Is(err, crew.ErrSessionRunning) {
			skipped = append(skipped, name)
			sessions = append(sessions, crewMgr.SessionName(name))
			continue
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			continue
		}
		sessions = append(sessions, crewMgr.SessionName(name))
	}

	if len(sessions) > 0 {
		output = append(output, fmt.Sprintf("Started: %s", strings.Join(sessions, ", ")))
	}
	if len(skipped) > 0 {
		output = append(output, fmt.Sprintf("Already running: %s", strings.Join(skipped, ", ")))
	}
	if len(failed) > 0 {
		output = append(output, fmt.Sprintf("Failed: %s", strings.Join(failed, ", ")))
	}

	return CrewActionResponse{
		Success:  len(failed) < len(names),
		Output:   strings.Join(output, "\n"),
		Sessions: sessions,
	}, nil
}

func (h *GUIHandler) runCrewStop(req CrewActionRequest) (CrewActionResponse, error) {
	if req.Rig == "" && !req.All {
		return CrewActionResponse{}, errors.New("rig is required")
	}

	var targets []CrewStatusItem
	if req.All && req.Rig == "" {
		targets = h.listRunningCrew("")
	} else {
		crewMgr, r, err := h.getCrewManager(req.Rig)
		if err != nil {
			return CrewActionResponse{}, err
		}

		names, err := h.resolveCrewTargets(req, crewMgr)
		if err != nil {
			return CrewActionResponse{}, err
		}

		for _, name := range names {
			targets = append(targets, CrewStatusItem{
				Name: r.Name + "/" + name,
				Rig:  r.Name,
			})
		}
	}

	if len(targets) == 0 {
		return CrewActionResponse{Success: true, Output: "No running crew sessions found."}, nil
	}

	t := tmux.NewTmux()
	var output []string
	var failed []string

	for _, target := range targets {
		rigName := target.Rig
		crewName := strings.TrimPrefix(target.Name, rigName+"/")
		if crewName == target.Name {
			crewName = target.Name
		}

		sessionID := crewSessionName(rigName, crewName)
		if hasSession, _ := t.HasSession(sessionID); !hasSession {
			continue
		}
		if err := t.KillSessionWithProcesses(sessionID); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", sessionID, err))
			continue
		}

		output = append(output, fmt.Sprintf("Stopped %s", sessionID))

		if townRoot, _ := workspace.FindFromCwd(); townRoot != "" {
			agent := fmt.Sprintf("%s/crew/%s", rigName, crewName)
			logger := townlog.NewLogger(townRoot)
			_ = logger.Log(townlog.EventKill, agent, "gt crew stop (web)")
		}
	}

	if len(failed) > 0 {
		output = append(output, fmt.Sprintf("Failed: %s", strings.Join(failed, ", ")))
	}

	return CrewActionResponse{
		Success: len(failed) == 0,
		Output:  strings.Join(output, "\n"),
	}, nil
}

func (h *GUIHandler) runCrewRestart(req CrewActionRequest) (CrewActionResponse, error) {
	if req.Rig == "" && !req.All {
		return CrewActionResponse{}, errors.New("rig is required")
	}

	var targets []CrewStatusItem
	if req.All && req.Rig == "" {
		targets = h.listRunningCrew("")
	} else {
		crewMgr, r, err := h.getCrewManager(req.Rig)
		if err != nil {
			return CrewActionResponse{}, err
		}
		names, err := h.resolveCrewTargets(req, crewMgr)
		if err != nil {
			return CrewActionResponse{}, err
		}
		for _, name := range names {
			targets = append(targets, CrewStatusItem{
				Name: r.Name + "/" + name,
				Rig:  r.Name,
			})
		}
	}

	if len(targets) == 0 {
		return CrewActionResponse{Success: true, Output: "No running crew sessions to restart."}, nil
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return CrewActionResponse{}, err
	}
	claudeConfigDir, _, _ := resolveAccountConfigDir(townRoot, req.Account)

	var output []string
	var failed []string
	var sessions []string

	for _, target := range targets {
		rigName := target.Rig
		crewName := strings.TrimPrefix(target.Name, rigName+"/")

		crewMgr, r, err := h.getCrewManager(rigName)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s/%s (%v)", rigName, crewName, err))
			continue
		}

		opts := crew.StartOptions{
			KillExisting:    true,
			Topic:           "restart",
			ClaudeConfigDir: claudeConfigDir,
			AgentOverride:   req.AgentOverride,
		}
		if err := crewMgr.Start(crewName, opts); err != nil {
			failed = append(failed, fmt.Sprintf("%s/%s (%v)", r.Name, crewName, err))
			continue
		}
		sessions = append(sessions, crewMgr.SessionName(crewName))
	}

	if len(sessions) > 0 {
		output = append(output, fmt.Sprintf("Restarted: %s", strings.Join(sessions, ", ")))
	}
	if len(failed) > 0 {
		output = append(output, fmt.Sprintf("Failed: %s", strings.Join(failed, ", ")))
	}

	return CrewActionResponse{
		Success:  len(failed) == 0,
		Output:   strings.Join(output, "\n"),
		Sessions: sessions,
	}, nil
}

func (h *GUIHandler) runCrewRefresh(req CrewActionRequest) (CrewActionResponse, error) {
	if req.Rig == "" || req.Name == "" {
		return CrewActionResponse{}, errors.New("rig and name are required")
	}

	_, r, err := h.resolveRig(req.Rig)
	if err != nil {
		return CrewActionResponse{}, err
	}

	crewMgr := crew.NewManager(r, git.NewGit(r.Path))
	worker, err := crewMgr.Get(req.Name)
	if err != nil {
		return CrewActionResponse{}, fmt.Errorf("getting crew worker: %w", err)
	}

	handoffMsg := strings.TrimSpace(req.Message)
	if handoffMsg == "" {
		handoffMsg = fmt.Sprintf("Context refresh for %s. Check mail and beads for current work state.", req.Name)
	}

	mailDir := filepath.Join(worker.ClonePath, "mail")
	if _, err := os.Stat(mailDir); os.IsNotExist(err) {
		if err := os.MkdirAll(mailDir, 0755); err != nil {
			return CrewActionResponse{}, fmt.Errorf("creating mail dir: %w", err)
		}
	}

	mailbox := mail.NewMailbox(mailDir)
	msg := &mail.Message{
		From:    fmt.Sprintf("%s/%s", r.Name, req.Name),
		To:      fmt.Sprintf("%s/%s", r.Name, req.Name),
		Subject: "🤝 HANDOFF: Context Refresh",
		Body:    handoffMsg,
	}
	if err := mailbox.Append(msg); err != nil {
		return CrewActionResponse{}, fmt.Errorf("sending handoff mail: %w", err)
	}

	if err := crewMgr.Start(req.Name, crew.StartOptions{
		KillExisting:  true,
		Topic:         "refresh",
		Interactive:   true,
		AgentOverride: req.AgentOverride,
	}); err != nil {
		return CrewActionResponse{}, fmt.Errorf("starting crew session: %w", err)
	}

	sessionID := crewMgr.SessionName(req.Name)
	output := fmt.Sprintf("Refreshed crew workspace: %s/%s", r.Name, req.Name)
	return CrewActionResponse{
		Success:  true,
		Output:   output,
		Sessions: []string{sessionID},
	}, nil
}

func (h *GUIHandler) runCrewPristine(req CrewActionRequest) (CrewActionResponse, error) {
	if req.Rig == "" {
		return CrewActionResponse{}, errors.New("rig is required")
	}

	crewMgr, _, err := h.getCrewManager(req.Rig)
	if err != nil {
		return CrewActionResponse{}, err
	}

	names, err := h.resolveCrewTargets(req, crewMgr)
	if err != nil {
		return CrewActionResponse{}, err
	}

	var output []string
	var failed []string

	for _, name := range names {
		result, err := crewMgr.Pristine(name)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
			continue
		}

		output = append(output, fmt.Sprintf("Pristine %s", result.Name))
		if result.HadChanges {
			output = append(output, "  Has uncommitted changes")
		}
		if result.Pulled {
			output = append(output, "  git pull: ok")
		} else if result.PullError != "" {
			output = append(output, fmt.Sprintf("  git pull: %s", result.PullError))
		}
		if result.Synced {
			output = append(output, "  bd sync: ok")
		} else if result.SyncError != "" {
			output = append(output, fmt.Sprintf("  bd sync: %s", result.SyncError))
		}
	}

	if len(failed) > 0 {
		output = append(output, fmt.Sprintf("Failed: %s", strings.Join(failed, ", ")))
	}

	return CrewActionResponse{
		Success: len(failed) < len(names),
		Output:  strings.Join(output, "\n"),
	}, nil
}

func (h *GUIHandler) discoverRigs() ([]*rig.Rig, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	manager := rig.NewManager(townRoot, rigsConfig, git.NewGit(townRoot))
	rigs, err := manager.DiscoverRigs()
	if err != nil {
		return nil, err
	}

	return rigs, nil
}

func (h *GUIHandler) resolveRig(rigName string) (string, *rig.Rig, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return "", nil, fmt.Errorf("loading rigs config: %w", err)
	}

	manager := rig.NewManager(townRoot, rigsConfig, git.NewGit(townRoot))
	r, err := manager.GetRig(rigName)
	if err != nil {
		return "", nil, fmt.Errorf("rig %q not found", rigName)
	}
	return townRoot, r, nil
}

func (h *GUIHandler) getCrewManager(rigName string) (*crew.Manager, *rig.Rig, error) {
	_, r, err := h.resolveRig(rigName)
	if err != nil {
		return nil, nil, err
	}

	crewMgr := crew.NewManager(r, git.NewGit(r.Path))
	return crewMgr, r, nil
}

func (h *GUIHandler) resolveCrewTargets(req CrewActionRequest, crewMgr *crew.Manager) ([]string, error) {
	names := crewNamesFromRequest(req)
	if req.All || len(names) == 0 {
		workers, err := crewMgr.List()
		if err != nil {
			return nil, fmt.Errorf("listing crew: %w", err)
		}
		if len(workers) == 0 {
			return nil, errors.New("no crew workers found")
		}
		names = names[:0]
		for _, w := range workers {
			names = append(names, w.Name)
		}
	}
	return names, nil
}

func crewNamesFromRequest(req CrewActionRequest) []string {
	if len(req.Names) > 0 {
		return req.Names
	}
	if req.Name != "" {
		return []string{req.Name}
	}
	return nil
}

func resolveAccountConfigDir(townRoot, account string) (string, string, error) {
	accountsPath := constants.MayorAccountsPath(townRoot)
	return config.ResolveAccountConfigDir(accountsPath, account)
}

func crewSessionName(rigName, crewName string) string {
	return fmt.Sprintf("gt-%s-crew-%s", rigName, crewName)
}

func (h *GUIHandler) listRunningCrew(rigFilter string) []CrewStatusItem {
	agents, err := h.fetcher.FetchAgents()
	if err != nil {
		return nil
	}
	var targets []CrewStatusItem
	for _, agent := range agents {
		if agent.AgentType != "crew" {
			continue
		}
		if rigFilter != "" && agent.Rig != rigFilter {
			continue
		}
		targets = append(targets, CrewStatusItem{
			Name: agent.Name,
			Rig:  agent.Rig,
		})
	}
	return targets
}
