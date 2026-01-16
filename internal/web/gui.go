package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed templates/dashboard.html
var dashboardHTML embed.FS

// GUIHandler handles the main Gas Town web GUI.
type GUIHandler struct {
	fetcher ConvoyFetcher
	mux     *http.ServeMux
}

// NewGUIHandler creates a new GUI handler with all routes.
func NewGUIHandler(fetcher ConvoyFetcher) (*GUIHandler, error) {
	h := &GUIHandler{
		fetcher: fetcher,
		mux:     http.NewServeMux(),
	}

	// Setup routes
	h.mux.HandleFunc("/", h.handleDashboard)
	h.mux.HandleFunc("/api/status", h.handleAPIStatus)
	h.mux.HandleFunc("/ws/status", h.handleStatusWS)
	h.mux.HandleFunc("/api/mail/send", h.handleAPISendMail)
	h.mux.HandleFunc("/api/mail/inbox", h.handleAPIMailInbox)
	h.mux.HandleFunc("/api/mail/all", h.handleAPIMailAll)
	h.mux.HandleFunc("/api/agents/list", h.handleAPIAgentsList)
	h.mux.HandleFunc("/api/command", h.handleAPICommand)
	h.mux.HandleFunc("/api/rigs", h.handleAPIRigs)
	h.mux.HandleFunc("/api/convoys", h.handleAPIConvoys)
	h.mux.HandleFunc("/api/terminal/stream", h.handleAPITerminalStream)
	h.mux.HandleFunc("/api/mayor/terminal", h.handleAPIMayorTerminal)
	h.mux.HandleFunc("/api/mayor/status", h.handleAPIMayorStatus)

	return h, nil
}

// handleAPIMayorTerminal streams the mayor's tmux session output
func (h *GUIHandler) handleAPIMayorTerminal(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastFrame := ""
	noChangeCount := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			frame, err := captureTmuxPane("hq-mayor")
			if err != nil {
				writeSSE(w, "error", err.Error())
				flusher.Flush()
				return
			}
			frame = strings.TrimRight(frame, "\n")
			if frame == lastFrame {
				noChangeCount++
				// Send keepalive every 15 seconds to prevent mobile timeout
				if noChangeCount >= 15 {
					writeSSE(w, "ping", "keepalive")
					flusher.Flush()
					noChangeCount = 0
				}
				continue
			}
			noChangeCount = 0
			lastFrame = frame
			writeSSE(w, "frame", frame)
			flusher.Flush()
		}
	}
}

// handleAPIMayorStatus returns mayor session status
func (h *GUIHandler) handleAPIMayorStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if mayor session exists
	cmd := exec.Command("tmux", "has-session", "-t", "hq-mayor")
	sessionExists := cmd.Run() == nil

	// Get hook status
	hookCmd := exec.Command("gt", "hook")
	hookOutput, _ := hookCmd.Output()

	// Get mail count
	mailCmd := exec.Command("gt", "mail", "inbox", "mayor/", "--json")
	mailOutput, _ := mailCmd.Output()

	var mailCount int
	var messages []interface{}
	if err := json.Unmarshal(mailOutput, &messages); err == nil {
		mailCount = len(messages)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_exists": sessionExists,
		"session_name":   "hq-mayor",
		"hook":           strings.TrimSpace(string(hookOutput)),
		"mail_count":     mailCount,
	})
}

func (h *GUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// StatusResponse represents the full town status.
type StatusResponse struct {
	Timestamp  time.Time       `json:"timestamp"`
	Daemon     DaemonStatus    `json:"daemon"`
	Rigs       []RigStatus     `json:"rigs"`
	Convoys    []ConvoyRow     `json:"convoys"`
	MergeQueue []MergeQueueRow `json:"merge_queue"`
	Polecats   []PolecatRow    `json:"polecats"`
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

func (h *GUIHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// SPA routing: serve the dashboard HTML for all non-API paths
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws/") {
		http.NotFound(w, r)
		return
	}

	content, err := dashboardHTML.ReadFile("templates/dashboard.html")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

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

func (h *GUIHandler) buildStatus() StatusResponse {
	status := StatusResponse{
		Timestamp: time.Now(),
	}

	// Get daemon status
	status.Daemon = h.getDaemonStatus()

	// Get rigs
	status.Rigs = h.getRigs()

	// Get convoys
	if convoys, err := h.fetcher.FetchConvoys(); err == nil {
		status.Convoys = convoys
	}

	// Get merge queue
	if mq, err := h.fetcher.FetchMergeQueue(); err == nil {
		status.MergeQueue = mq
	}

	// Get polecats
	if polecats, err := h.fetcher.FetchPolecats(); err == nil {
		status.Polecats = polecats
	}

	// Get mail status
	status.Mail = h.getMailStatus()

	return status
}

func (h *GUIHandler) handleAPISendMail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Send via gt mail send
	args := []string{"mail", "send", req.To, "-s", req.Subject}
	if req.Body != "" {
		args = append(args, "-m", req.Body)
	}

	cmd := exec.Command("gt", args...)
	output, err := cmd.CombinedOutput()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   string(output),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"output":  string(output),
	})
}

func (h *GUIHandler) handleAPIMailInbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cmd := exec.Command("gt", "mail", "inbox", "--json")
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"messages": []interface{}{},
			"error":    err.Error(),
		})
		return
	}

	w.Write(output)
}

// handleAPIMailAll gets mail for any agent
func (h *GUIHandler) handleAPIMailAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agent := r.URL.Query().Get("agent")
	if agent == "" {
		agent = "mayor/"
	}

	// Get inbox for specific agent
	cmd := exec.Command("gt", "mail", "inbox", agent, "--json")
	output, err := cmd.Output()
	if err != nil {
		// Try without --json if it fails
		cmd2 := exec.Command("gt", "mail", "inbox", agent)
		output2, _ := cmd2.CombinedOutput()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agent":   agent,
			"raw":     string(output2),
			"error":   err.Error(),
		})
		return
	}

	// Parse and forward the JSON
	var messages interface{}
	if err := json.Unmarshal(output, &messages); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agent": agent,
			"raw":   string(output),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"agent":    agent,
		"messages": messages,
	})
}

// handleAPIAgentsList returns all available agents for mail recipients
func (h *GUIHandler) handleAPIAgentsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agents := []map[string]string{
		{"address": "mayor/", "name": "Mayor", "type": "mayor"},
		{"address": "deacon/", "name": "Deacon", "type": "deacon"},
	}

	// Get crew from all rigs
	cmd := exec.Command("gt", "crew", "list", "--all")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			// Parse lines like "  ● gastown/flux"
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "●") || strings.HasPrefix(line, "○") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					name := parts[1]
					agents = append(agents, map[string]string{
						"address": name + "/",
						"name":    name,
						"type":    "crew",
					})
				}
			}
		}
	}

	// Get polecats
	cmd = exec.Command("gt", "polecat", "list", "--all")
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "/") && !strings.HasPrefix(line, "No") {
				parts := strings.Fields(line)
				if len(parts) >= 1 {
					name := parts[0]
					agents = append(agents, map[string]string{
						"address": name + "/",
						"name":    name,
						"type":    "polecat",
					})
				}
			}
		}
	}

	// Add witness and refinery for each rig
	rigs := h.getRigs()
	for _, rig := range rigs {
		agents = append(agents,
			map[string]string{"address": rig.Name + "/witness/", "name": rig.Name + " Witness", "type": "witness"},
			map[string]string{"address": rig.Name + "/refinery/", "name": rig.Name + " Refinery", "type": "refinery"},
		)
	}

	json.NewEncoder(w).Encode(agents)
}

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
	cmd := exec.Command("gt", args...)
	output, err := cmd.CombinedOutput()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": err == nil,
		"output":  string(output),
		"error":   err != nil,
	})
}

func (h *GUIHandler) handleAPIRigs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rigs := h.getRigs()
	json.NewEncoder(w).Encode(rigs)
}

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

func (h *GUIHandler) getDaemonStatus() DaemonStatus {
	cmd := exec.Command("gt", "daemon", "status")
	output, err := cmd.CombinedOutput()

	return DaemonStatus{
		Running: err == nil && strings.Contains(string(output), "running"),
	}
}

func (h *GUIHandler) getRigs() []RigStatus {
	cmd := exec.Command("gt", "rig", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Parse text output (gt rig list doesn't have --json yet)
	// Format:
	//   Rigs in /path:
	//
	//     rigname
	//       Polecats: N  Crew: M
	//       Agents: [...]
	var rigs []RigStatus
	var currentRig *RigStatus
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Rigs in") || line == "" {
			continue
		}
		// Rig name: 2 spaces then name
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			name := strings.TrimSpace(line)
			if name != "" {
				rigs = append(rigs, RigStatus{Name: name})
				currentRig = &rigs[len(rigs)-1]
			}
		}
		// Rig details: 4 spaces then "Polecats: N  Crew: M"
		if strings.HasPrefix(line, "    ") && currentRig != nil {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Polecats:") {
				var polecats, crew int
				fmt.Sscanf(trimmed, "Polecats: %d  Crew: %d", &polecats, &crew)
				currentRig.Polecats = polecats
				currentRig.Crew = crew
			}
		}
	}
	return rigs
}

func (h *GUIHandler) getMailStatus() MailStatus {
	cmd := exec.Command("gt", "mail", "inbox")
	output, err := cmd.Output()
	if err != nil {
		return MailStatus{}
	}

	// Parse output for unread count
	outStr := string(output)
	var unread int
	if strings.Contains(outStr, "unread") {
		// Extract number from "N unread"
		parts := strings.Fields(outStr)
		for i, p := range parts {
			if p == "unread" && i > 0 {
				var n int
				if _, err := strings.NewReader(parts[i-1]).Read([]byte{byte(n)}); err == nil {
					unread = n
				}
			}
		}
	}

	return MailStatus{
		Unread: unread,
	}
}

// tmuxSessionNamePattern matches polecat sessions (gt-rig-name) and HQ sessions (hq-mayor, hq-deacon)
var tmuxSessionNamePattern = regexp.MustCompile(`^(gt-[a-zA-Z0-9_-]+-[a-zA-Z0-9_-]+|hq-(mayor|deacon))$`)
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func (h *GUIHandler) handleAPITerminalStream(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	if session == "" || !tmuxSessionNamePattern.MatchString(session) {
		http.Error(w, "Invalid session", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastFrame := ""
	noChangeCount := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			frame, err := captureTmuxPane(session)
			if err != nil {
				writeSSE(w, "error", err.Error())
				flusher.Flush()
				return
			}
			frame = strings.TrimRight(frame, "\n")
			if frame == lastFrame {
				noChangeCount++
				// Send keepalive every 15 seconds to prevent mobile timeout
				if noChangeCount >= 15 {
					writeSSE(w, "ping", "keepalive")
					flusher.Flush()
					noChangeCount = 0
				}
				continue
			}
			noChangeCount = 0
			lastFrame = frame
			writeSSE(w, "frame", frame)
			flusher.Flush()
		}
	}
}

func captureTmuxPane(session string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-J", "-S", "-2000")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return sanitizeTerminalOutput(string(output)), nil
}

func sanitizeTerminalOutput(s string) string {
	s = ansiEscapePattern.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return r
		default:
			if r < 32 {
				return -1
			}
			return r
		}
	}, s)
}

func writeSSE(w http.ResponseWriter, event, data string) {
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}
