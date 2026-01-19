package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// DashboardPageData is the data passed to the dashboard template.
type DashboardPageData struct {
	Title      string
	ActivePage string
}

// handleDashboard serves the dashboard page.
func (h *GUIHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" path, not all paths
	if r.URL.Path != "/" {
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
		return true
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
	cmd := exec.Command("gt", "daemon", "status")
	output, err := cmd.CombinedOutput()

	return DaemonStatus{
		Running: err == nil && strings.Contains(string(output), "running"),
	}
}

func (h *GUIHandler) getMailStatus() MailStatus {
	cmd := exec.Command("gt", "mail", "inbox")
	output, err := cmd.Output()
	if err != nil {
		return MailStatus{}
	}

	// Parse output for unread count
	// Format: "📬 Inbox: mayor/ (N messages, M unread)"
	outStr := string(output)
	var unread, total int

	// Look for pattern like "12 messages" and "5 unread"
	parts := strings.Fields(outStr)
	for i, p := range parts {
		if p == "messages," && i > 0 {
			if n, err := parseInt(parts[i-1]); err == nil {
				total = n
			}
		}
		if p == "unread)" && i > 0 {
			if n, err := parseInt(parts[i-1]); err == nil {
				unread = n
			}
		}
	}

	return MailStatus{
		Unread: unread,
		Total:  total,
	}
}

// parseInt extracts an integer from a string, stripping non-numeric prefix like "("
func parseInt(s string) (int, error) {
	s = strings.TrimLeft(s, "(")
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	if n == 0 && s != "0" {
		return 0, fmt.Errorf("no number found")
	}
	return n, nil
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
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority int    `json:"priority"`
	Type     string `json:"issue_type"`
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
	args := []string{"list", "--json"}
	if status != "" {
		args = append(args, "--status="+status)
	}

	cmd := exec.Command("bd", args...)
	output, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{
			"error":  "Failed to fetch issues",
			"issues": []IssueRow{},
		}
	}

	var issues []IssueRow
	if err := json.Unmarshal(output, &issues); err != nil {
		return map[string]interface{}{
			"error":  "Failed to parse issues",
			"issues": []IssueRow{},
		}
	}

	// Limit to first 20 issues for dashboard
	if len(issues) > 20 {
		issues = issues[:20]
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
	// Query beads with issue_type='agent' and status='open'
	args := []string{"list", "--type=agent", "--status=open", "--json"}

	cmd := exec.Command("bd", args...)
	output, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{
			"error":      "Failed to fetch role beads",
			"role_beads": []RoleBead{},
		}
	}

	var beads []struct {
		ID        string            `json:"id"`
		Title     string            `json:"title"`
		Status    string            `json:"status"`
		Labels    []string          `json:"labels"`
		CreatedAt string            `json:"created_at"`
		Metadata  map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(output, &beads); err != nil {
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

		roleBeads = append(roleBeads, RoleBead{
			ID:        b.ID,
			Title:     b.Title,
			Status:    b.Status,
			RoleType:  roleType,
			CreatedAt: b.CreatedAt,
		})
	}

	return map[string]interface{}{
		"role_beads": roleBeads,
	}
}
