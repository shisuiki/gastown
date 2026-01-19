package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/workspace"
)

// DashboardPageData is the data passed to the dashboard template.
type DashboardPageData struct {
	Title      string
	ActivePage string
}

// handleDashboard serves the dashboard page.
func (h *GUIHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" and "/dashboard" paths
	if r.URL.Path != "/" && r.URL.Path != "/dashboard" {
		http.NotFound(w, r)
		return
	}

	data := DashboardPageData{
		Title:      "Dashboard",
		ActivePage: "dashboard",
	}

	h.renderTemplate(w, "dashboard.html", data)
}

// handleAPIStatus returns the full status JSON.
func (h *GUIHandler) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := h.buildStatus()
	json.NewEncoder(w).Encode(status)
}

var statusWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return isSameOriginRequest(r)
	},
}

// handleStatusWS provides WebSocket status updates.
func (h *GUIHandler) handleStatusWS(w http.ResponseWriter, r *http.Request) {
	conn, err := statusWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	sendStatus := func() error {
		status := h.buildStatus()
		return conn.WriteJSON(status)
	}

	if err := sendStatus(); err != nil {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := sendStatus(); err != nil {
				return
			}
		}
	}
}

// StatusResponse represents the full town status.
type StatusResponse struct {
	Timestamp  time.Time       `json:"timestamp"`
	Daemon     DaemonStatus    `json:"daemon"`
	Rigs       []RigStatus     `json:"rigs"`
	Convoys    []ConvoyRow     `json:"convoys"`
	MergeQueue []MergeQueueRow `json:"merge_queue"`
	Agents     []AgentRow      `json:"agents"`
	Mail       MailStatus      `json:"mail"`
}

// DaemonStatus represents daemon health.
type DaemonStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Uptime  string `json:"uptime,omitempty"`
}

// RigStatus represents a rig's status.
type RigStatus struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Polecats   int    `json:"polecats"`
	Crew       int    `json:"crew"`
	HasWitness bool   `json:"has_witness"`
}

// MailStatus represents mail queue status.
type MailStatus struct {
	Unread int `json:"unread"`
	Total  int `json:"total"`
}

func (h *GUIHandler) buildStatus() StatusResponse {
	// Use cached status if available
	return h.statusCache.GetOrBuild(h.buildStatusUncached)
}

func (h *GUIHandler) buildStatusUncached() StatusResponse {
	status := StatusResponse{
		Timestamp: time.Now(),
	}

	// Get daemon status (fast, no caching needed)
	status.Daemon = h.getDaemonStatus()

	// Get rigs (fast enough, no caching needed)
	status.Rigs = h.getRigs()

	// Get convoys (expensive - uses its own cache in fetcher)
	if convoys, err := h.fetcher.FetchConvoys(); err == nil {
		status.Convoys = convoys
	}

	// Get merge queue
	if mq, err := h.fetcher.FetchMergeQueue(); err == nil {
		status.MergeQueue = mq
	}

	// Get agents (expensive - uses its own cache in fetcher)
	if agents, err := h.fetcher.FetchAgents(); err == nil {
		status.Agents = agents
	}

	// Get mail status (medium cost)
	status.Mail = h.getMailStatus()

	return status
}

func (h *GUIHandler) getDaemonStatus() DaemonStatus {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return DaemonStatus{}
	}

	running, pid, err := daemon.IsRunning(townRoot)
	if err != nil {
		return DaemonStatus{}
	}

	status := DaemonStatus{
		Running: running,
		PID:     pid,
	}

	if running {
		if state, err := daemon.LoadState(townRoot); err == nil && !state.StartedAt.IsZero() {
			uptime := time.Since(state.StartedAt).Truncate(time.Second)
			if uptime < 0 {
				uptime = 0
			}
			status.Uptime = uptime.String()
		}
	}

	return status
}

func (h *GUIHandler) getMailStatus() MailStatus {
	router, err := h.mailRouter()
	if err != nil {
		return MailStatus{}
	}

	mailbox, err := router.GetMailbox("mayor/")
	if err != nil {
		return MailStatus{}
	}

	total, unread, err := mailbox.Count()
	if err != nil {
		return MailStatus{}
	}

	return MailStatus{
		Unread: unread,
		Total:  total,
	}
}

// renderTemplate renders a template with the layout.
func (h *GUIHandler) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	tmpl, err := LoadTemplates()
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// IssueRow represents an issue in the dashboard.
type IssueRow struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Priority int      `json:"priority"`
	Type     string   `json:"issue_type"`
	Assignee string   `json:"assignee,omitempty"`
	Labels   []string `json:"labels,omitempty"`
	Wisp     bool     `json:"wisp,omitempty"`
}

// RoleBead represents an agent lifecycle bead.
type RoleBead struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	RoleType  string `json:"role_type"`
	CreatedAt string `json:"created_at"`
}

// handleAPIIssues returns issues from beads.
func (h *GUIHandler) handleAPIIssues(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get status filter from query params
	status := r.URL.Query().Get("status")
	cacheKey := "issues_" + status

	// Use stale-while-revalidate
	cached := h.cache.GetStaleOrRefresh(cacheKey, IssuesCacheTTL, func() interface{} {
		return h.fetchIssues(status)
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchIssues(status)
	h.cache.Set(cacheKey, result, IssuesCacheTTL)
	json.NewEncoder(w).Encode(result)
}

// fetchIssues gets issues from beads.
func (h *GUIHandler) fetchIssues(status string) map[string]interface{} {
	reader, err := NewBeadsReader("")
	if err != nil {
		return map[string]interface{}{
			"error":  "Failed to fetch issues",
			"issues": []IssueRow{},
		}
	}

	beads, err := reader.ListBeads(BeadFilter{
		Status:           status,
		IncludeEphemeral: true,
		Limit:            20,
	})
	if err != nil {
		return map[string]interface{}{
			"error":  "Failed to parse issues",
			"issues": []IssueRow{},
		}
	}

	issues := make([]IssueRow, 0, len(beads))
	for _, bead := range beads {
		issue := IssueRow{
			ID:       bead.ID,
			Title:    bead.Title,
			Status:   bead.Status,
			Priority: bead.Priority,
			Type:     bead.Type,
			Assignee: bead.Assignee,
			Labels:   bead.Labels,
			Wisp:     bead.Ephemeral,
		}

		if !issue.Wisp {
			for _, label := range bead.Labels {
				if label == "ephemeral" || label == "wisp" {
					issue.Wisp = true
					break
				}
			}
		}

		issues = append(issues, issue)
	}

	return map[string]interface{}{
		"issues": issues,
	}
}

// handleAPIRoleBeads returns role beads (agent lifecycle beads) from the database.
func (h *GUIHandler) handleAPIRoleBeads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cacheKey := "role_beads"

	// Use stale-while-revalidate
	cached := h.cache.GetStaleOrRefresh(cacheKey, IssuesCacheTTL, func() interface{} {
		return h.fetchRoleBeads()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchRoleBeads()
	h.cache.Set(cacheKey, result, IssuesCacheTTL)
	json.NewEncoder(w).Encode(result)
}

// fetchRoleBeads gets role beads (issue_type='agent') from beads.
func (h *GUIHandler) fetchRoleBeads() map[string]interface{} {
	reader, err := NewBeadsReader("")
	if err != nil {
		return map[string]interface{}{
			"error":      "Failed to fetch role beads",
			"role_beads": []RoleBead{},
		}
	}

	beads, err := reader.ListBeads(BeadFilter{
		Status:           "open",
		Type:             "agent",
		IncludeEphemeral: true,
	})
	if err != nil {
		return map[string]interface{}{
			"error":      "Failed to parse role beads",
			"role_beads": []RoleBead{},
		}
	}

	// Convert to RoleBead format
	roleBeads := make([]RoleBead, 0, len(beads))
	for _, b := range beads {
		// Extract role_type from labels (format: role_type:xxx)
		roleType := "agent"
		for _, label := range b.Labels {
			if strings.HasPrefix(label, "role_type:") {
				roleType = strings.TrimPrefix(label, "role_type:")
				break
			}
		}

		createdAt := ""
		if !b.CreatedAt.IsZero() {
			createdAt = b.CreatedAt.Format(time.RFC3339)
		}

		roleBeads = append(roleBeads, RoleBead{
			ID:        b.ID,
			Title:     b.Title,
			Status:    b.Status,
			RoleType:  roleType,
			CreatedAt: createdAt,
		})
	}

	return map[string]interface{}{
		"role_beads": roleBeads,
	}
}
