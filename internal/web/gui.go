package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

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
				continue
			}
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
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
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
				continue
			}
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

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Gas Town Control Panel</title>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, monospace;
            background: #1a1a2e;
            color: #eee;
            min-height: 100vh;
        }
        .container { max-width: 1400px; margin: 0 auto; padding: 20px; }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 20px 0;
            border-bottom: 1px solid #333;
            margin-bottom: 20px;
        }
        h1 { font-size: 1.5rem; display: flex; align-items: center; gap: 10px; }
        h1::before { content: '🏭'; }
        .status-indicator {
            display: inline-block;
            width: 12px;
            height: 12px;
            border-radius: 50%;
            margin-left: 10px;
        }
        .status-green { background: #4ade80; box-shadow: 0 0 10px #4ade80; }
        .status-yellow { background: #fbbf24; box-shadow: 0 0 10px #fbbf24; }
        .status-red { background: #f87171; box-shadow: 0 0 10px #f87171; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(400px, 1fr)); gap: 20px; }
        .card {
            background: #16213e;
            border-radius: 12px;
            padding: 20px;
            border: 1px solid #333;
        }
        .card h2 {
            font-size: 1rem;
            color: #94a3b8;
            margin-bottom: 15px;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .card-content { font-size: 0.9rem; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #333; }
        th { color: #64748b; font-weight: 500; }
        .chat-container {
            grid-column: 1 / -1;
            display: flex;
            flex-direction: column;
            height: 400px;
        }
        .chat-messages {
            flex: 1;
            overflow-y: auto;
            padding: 15px;
            background: #0f172a;
            border-radius: 8px;
            margin-bottom: 10px;
        }
        .message {
            padding: 10px 15px;
            margin: 5px 0;
            border-radius: 8px;
            max-width: 80%;
        }
        .message.sent {
            background: #3b82f6;
            margin-left: auto;
        }
        .message.received {
            background: #334155;
        }
        .chat-input {
            display: flex;
            gap: 10px;
        }
        .chat-input input {
            flex: 1;
            padding: 12px 15px;
            border: 1px solid #333;
            border-radius: 8px;
            background: #0f172a;
            color: #fff;
            font-size: 0.9rem;
        }
        .chat-input button {
            padding: 12px 24px;
            background: #3b82f6;
            color: white;
            border: none;
            border-radius: 8px;
            cursor: pointer;
            font-weight: 500;
        }
        .chat-input button:hover { background: #2563eb; }
        .badge {
            display: inline-block;
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 500;
        }
        .badge-green { background: #166534; color: #4ade80; }
        .badge-yellow { background: #854d0e; color: #fbbf24; }
        .badge-red { background: #991b1b; color: #f87171; }
        .badge-blue { background: #1e40af; color: #60a5fa; }
        #status-time { color: #64748b; font-size: 0.8rem; }
        .terminal-container {
            grid-column: 1 / -1;
            display: flex;
            flex-direction: column;
            gap: 12px;
        }
        .terminal-controls {
            display: flex;
            gap: 10px;
            align-items: center;
        }
        .terminal-controls select {
            flex: 1;
            padding: 10px 12px;
            border-radius: 8px;
            border: 1px solid #333;
            background: #0f172a;
            color: #e2e8f0;
        }
        .terminal-controls button {
            padding: 10px 18px;
            border-radius: 8px;
            border: none;
            background: #22c55e;
            color: #0b1220;
            font-weight: 600;
            cursor: pointer;
        }
        .terminal-controls button.disconnect {
            background: #f97316;
            color: #0b1220;
        }
        .terminal-output {
            background: #0b1120;
            color: #e2e8f0;
            border-radius: 10px;
            padding: 16px;
            min-height: 220px;
            max-height: 420px;
            overflow-y: auto;
            font-family: "Menlo", "Monaco", "Courier New", monospace;
            font-size: 0.85rem;
            line-height: 1.4;
            border: 1px solid #1f2a44;
            white-space: pre-wrap;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Gas Town <span class="status-indicator status-green" id="daemon-status"></span></h1>
            <span id="status-time">Loading...</span>
        </header>

        <!-- Mayor Control Panel - Primary Interface -->
        <div class="card" style="margin-bottom: 20px; border: 2px solid #3b82f6;">
            <h2>🏛️ Mayor Control <span class="status-indicator" id="mayor-status"></span> <span id="mayor-hook" style="font-size: 0.75rem; color: #64748b; margin-left: 10px;"></span></h2>
            <div style="display: grid; grid-template-columns: 1fr 350px; gap: 20px;">
                <div>
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
                        <span style="color: #64748b; font-size: 0.8rem;">Mayor Terminal (hq-mayor)</span>
                        <button id="mayor-terminal-toggle" onclick="toggleMayorTerminal()" style="padding: 6px 12px; background: #22c55e; color: #0b1220; border: none; border-radius: 6px; cursor: pointer; font-size: 0.8rem;">Connect</button>
                    </div>
                    <pre class="terminal-output" id="mayor-terminal" style="min-height: 300px; max-height: 400px;">Click "Connect" to view Mayor's terminal output.</pre>
                </div>
                <div style="display: flex; flex-direction: column; gap: 10px;">
                    <div style="color: #64748b; font-size: 0.8rem;">Send Mail to Mayor</div>
                    <input type="text" id="mayor-mail-subject" placeholder="Subject" style="padding: 10px; border-radius: 6px; border: 1px solid #333; background: #0f172a; color: #fff;" />
                    <textarea id="mayor-mail-body" placeholder="Message to Mayor..." rows="6" style="padding: 10px; border-radius: 6px; border: 1px solid #333; background: #0f172a; color: #fff; resize: vertical; flex: 1;"></textarea>
                    <button onclick="sendMailToMayor()" style="padding: 12px; background: #3b82f6; color: white; border: none; border-radius: 6px; cursor: pointer; font-weight: 500;">📨 Send to Mayor</button>
                    <div id="mayor-mail-result" style="font-size: 0.8rem; color: #64748b;"></div>
                </div>
            </div>
        </div>

        <div class="grid">
            <div class="card">
                <h2>📊 Rigs</h2>
                <div class="card-content" id="rigs-list">Loading...</div>
            </div>

            <div class="card">
                <h2>🚚 Convoys</h2>
                <div class="card-content" id="convoys-list">Loading...</div>
            </div>

            <div class="card">
                <h2>🐱 Polecats</h2>
                <div class="card-content" id="polecats-list">Loading...</div>
            </div>

            <div class="card" style="grid-column: 1 / -1;">
                <h2>📬 Mail Center</h2>
                <div class="card-content">
                    <div style="display: flex; gap: 20px; margin-bottom: 15px;">
                        <div style="flex: 1;">
                            <label style="color: #64748b; font-size: 0.8rem;">View Inbox For:</label>
                            <select id="mail-agent-select" onchange="loadAgentMail()" style="width: 100%; padding: 8px; border-radius: 6px; border: 1px solid #333; background: #0f172a; color: #fff; margin-top: 5px;">
                                <option value="mayor/">Mayor</option>
                            </select>
                        </div>
                        <div style="flex: 1;">
                            <label style="color: #64748b; font-size: 0.8rem;">Send To:</label>
                            <select id="mail-to-select" style="width: 100%; padding: 8px; border-radius: 6px; border: 1px solid #333; background: #0f172a; color: #fff; margin-top: 5px;">
                                <option value="mayor/">Mayor</option>
                            </select>
                        </div>
                    </div>
                    <div id="mail-inbox" style="background: #0f172a; border-radius: 8px; padding: 15px; max-height: 200px; overflow-y: auto; margin-bottom: 15px;">
                        <p style="color: #64748b;">Select an agent to view their inbox</p>
                    </div>
                    <div style="display: flex; flex-direction: column; gap: 10px;">
                        <input type="text" id="mail-subject" placeholder="Subject" style="padding: 10px; border-radius: 6px; border: 1px solid #333; background: #0f172a; color: #fff;" />
                        <textarea id="mail-body" placeholder="Message body..." rows="3" style="padding: 10px; border-radius: 6px; border: 1px solid #333; background: #0f172a; color: #fff; resize: vertical;"></textarea>
                        <button onclick="sendMailToAgent()" style="padding: 10px 20px; background: #3b82f6; color: white; border: none; border-radius: 6px; cursor: pointer; font-weight: 500;">Send Mail</button>
                    </div>
                </div>
            </div>

            <div class="card chat-container">
                <h2>💬 Quick Message to Mayor</h2>
                <div class="chat-messages" id="chat-messages"></div>
                <div class="chat-input">
                    <input type="text" id="chat-input" placeholder="Send a message to Mayor..." />
                    <button onclick="sendMessage()">Send</button>
                </div>
            </div>

            <div class="card terminal-container">
                <h2>🖥️ Polecat Terminal</h2>
                <div class="terminal-controls">
                    <select id="terminal-session"></select>
                    <button id="terminal-toggle" onclick="toggleTerminalStream()">Connect</button>
                </div>
                <pre class="terminal-output" id="terminal-output">Select a polecat session to view its terminal output.</pre>
            </div>
        </div>
    </div>

    <script>
        let terminalSource = null;
        let terminalConnected = false;
        function connectStatusSocket() {
            const protocol = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
            const socket = new WebSocket(protocol + window.location.host + '/ws/status');

            socket.addEventListener('message', (event) => {
                try {
                    const data = JSON.parse(event.data);
                    updateUI(data);
                } catch (e) {
                    console.error('Failed to parse status update:', e);
                }
            });

            socket.addEventListener('close', () => {
                document.getElementById('status-time').textContent = 'Disconnected';
                setTimeout(connectStatusSocket, 2000);
            });

            socket.addEventListener('error', () => {
                socket.close();
            });
        }

        function updateUI(data) {
            // Update timestamp
            document.getElementById('status-time').textContent =
                'Updated: ' + new Date(data.timestamp).toLocaleTimeString();

            // Update daemon status
            const daemonIndicator = document.getElementById('daemon-status');
            daemonIndicator.className = 'status-indicator ' +
                (data.daemon.running ? 'status-green' : 'status-red');

            // Update rigs
            const rigsHtml = data.rigs && data.rigs.length > 0
                ? '<table><tr><th>Name</th><th>Polecats</th><th>Crew</th></tr>' +
                  data.rigs.map(r => '<tr><td>' + r.name + '</td><td>' + (r.polecats||0) + '</td><td>' + (r.crew||0) + '</td></tr>').join('') +
                  '</table>'
                : '<p>No rigs configured</p>';
            document.getElementById('rigs-list').innerHTML = rigsHtml;

            // Update convoys
            const convoysHtml = data.convoys && data.convoys.length > 0
                ? '<table><tr><th>ID</th><th>Title</th><th>Progress</th></tr>' +
                  data.convoys.map(c => '<tr><td>' + c.ID + '</td><td>' + c.Title + '</td><td>' + c.Progress + '</td></tr>').join('') +
                  '</table>'
                : '<p>No active convoys</p>';
            document.getElementById('convoys-list').innerHTML = convoysHtml;

            // Update polecats
            const polecatsHtml = data.polecats && data.polecats.length > 0
                ? '<table><tr><th>Name</th><th>Rig</th><th>Activity</th></tr>' +
                  data.polecats.map(p => '<tr><td>' + p.Name + '</td><td>' + p.Rig + '</td><td><span class="badge badge-' + getColorClass(p.LastActivity.ColorClass) + '">' + p.LastActivity.FormattedAge + '</span></td></tr>').join('') +
                  '</table>'
                : '<p>No polecats running</p>';
            document.getElementById('polecats-list').innerHTML = polecatsHtml;
            updateTerminalSessions(data.polecats || []);

            // Update mail
            document.getElementById('mail-status').innerHTML =
                '<p>Unread: <strong>' + data.mail.unread + '</strong> / Total: ' + data.mail.total + '</p>';
        }

        function getColorClass(color) {
            if (color === 'green' || color === 'activity-green') return 'green';
            if (color === 'yellow' || color === 'activity-yellow') return 'yellow';
            if (color === 'red' || color === 'activity-red') return 'red';
            return 'blue';
        }

        async function sendMessage() {
            const input = document.getElementById('chat-input');
            const message = input.value.trim();
            if (!message) return;

            // Add sent message to chat
            addMessage(message, 'sent');
            input.value = '';

            try {
                const res = await fetch('/api/mail/send', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        to: 'mayor/',
                        subject: 'Web GUI Message',
                        body: message
                    })
                });
                const data = await res.json();
                if (data.success) {
                    addMessage('Message sent to Mayor', 'received');
                } else {
                    addMessage('Error: ' + data.error, 'received');
                }
            } catch (e) {
                addMessage('Failed to send: ' + e.message, 'received');
            }
        }

        function addMessage(text, type) {
            const messages = document.getElementById('chat-messages');
            const div = document.createElement('div');
            div.className = 'message ' + type;
            div.textContent = text;
            messages.appendChild(div);
            messages.scrollTop = messages.scrollHeight;
        }

        function updateTerminalSessions(polecats) {
            const select = document.getElementById('terminal-session');
            if (!select) return;

            const previous = select.value;
            const sessions = polecats
                .filter(p => p.SessionID)
                .map(p => ({
                    id: p.SessionID,
                    label: p.Rig + '/' + p.Name
                }));

            select.innerHTML = '';
            if (sessions.length === 0) {
                const option = document.createElement('option');
                option.value = '';
                option.textContent = 'No active polecat sessions';
                select.appendChild(option);
                return;
            }

            sessions.forEach(session => {
                const option = document.createElement('option');
                option.value = session.id;
                option.textContent = session.label + ' (' + session.id + ')';
                select.appendChild(option);
            });

            if (previous && sessions.some(s => s.id === previous)) {
                select.value = previous;
                return;
            }
            select.selectedIndex = 0;
        }

        function toggleTerminalStream() {
            if (terminalConnected) {
                disconnectTerminalStream();
            } else {
                connectTerminalStream();
            }
        }

        function connectTerminalStream() {
            const select = document.getElementById('terminal-session');
            const output = document.getElementById('terminal-output');
            const toggle = document.getElementById('terminal-toggle');
            const session = select.value;
            if (!session) {
                output.textContent = 'No session selected.';
                return;
            }

            if (terminalSource) {
                terminalSource.close();
            }

            output.textContent = 'Connecting to ' + session + '...';
            terminalSource = new EventSource('/api/terminal/stream?session=' + encodeURIComponent(session));
            terminalConnected = true;
            toggle.textContent = 'Disconnect';
            toggle.classList.add('disconnect');

            terminalSource.addEventListener('frame', (event) => {
                output.textContent = event.data;
                output.scrollTop = output.scrollHeight;
            });

            terminalSource.addEventListener('error', (event) => {
                if (event.data) {
                    output.textContent = 'Error: ' + event.data;
                } else {
                    output.textContent = 'Stream disconnected.';
                }
                disconnectTerminalStream();
            });
        }

        function disconnectTerminalStream() {
            const toggle = document.getElementById('terminal-toggle');
            if (terminalSource) {
                terminalSource.close();
                terminalSource = null;
            }
            terminalConnected = false;
            toggle.textContent = 'Connect';
            toggle.classList.remove('disconnect');
        }

        // Handle Enter key in chat input
        document.getElementById('chat-input').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') sendMessage();
        });

        // Mail Center functions
        async function loadAgentsList() {
            try {
                const res = await fetch('/api/agents/list');
                const agents = await res.json();
                const viewSelect = document.getElementById('mail-agent-select');
                const sendSelect = document.getElementById('mail-to-select');

                viewSelect.innerHTML = '';
                sendSelect.innerHTML = '';

                agents.forEach(agent => {
                    const opt1 = document.createElement('option');
                    opt1.value = agent.address;
                    opt1.textContent = agent.name + ' (' + agent.type + ')';
                    viewSelect.appendChild(opt1);

                    const opt2 = document.createElement('option');
                    opt2.value = agent.address;
                    opt2.textContent = agent.name + ' (' + agent.type + ')';
                    sendSelect.appendChild(opt2);
                });

                // Load mayor's mail by default
                loadAgentMail();
            } catch (e) {
                console.error('Failed to load agents:', e);
            }
        }

        async function loadAgentMail() {
            const agent = document.getElementById('mail-agent-select').value;
            const inbox = document.getElementById('mail-inbox');
            inbox.innerHTML = '<p style="color: #64748b;">Loading...</p>';

            try {
                const res = await fetch('/api/mail/all?agent=' + encodeURIComponent(agent));
                const data = await res.json();

                if (data.raw) {
                    inbox.innerHTML = '<pre style="white-space: pre-wrap; color: #e2e8f0; margin: 0;">' + escapeHtml(data.raw) + '</pre>';
                } else if (data.messages && Array.isArray(data.messages)) {
                    if (data.messages.length === 0) {
                        inbox.innerHTML = '<p style="color: #64748b;">No messages</p>';
                    } else {
                        inbox.innerHTML = data.messages.map(m =>
                            '<div style="border-bottom: 1px solid #333; padding: 8px 0;">' +
                            '<div style="font-weight: 500; color: #60a5fa;">' + escapeHtml(m.subject || 'No subject') + '</div>' +
                            '<div style="font-size: 0.8rem; color: #94a3b8;">From: ' + escapeHtml(m.from || 'unknown') + '</div>' +
                            '<div style="margin-top: 5px; color: #e2e8f0;">' + escapeHtml(m.body || m.content || '') + '</div>' +
                            '</div>'
                        ).join('');
                    }
                } else {
                    inbox.innerHTML = '<p style="color: #64748b;">No messages for ' + agent + '</p>';
                }
            } catch (e) {
                inbox.innerHTML = '<p style="color: #f87171;">Error: ' + e.message + '</p>';
            }
        }

        async function sendMailToAgent() {
            const to = document.getElementById('mail-to-select').value;
            const subject = document.getElementById('mail-subject').value.trim();
            const body = document.getElementById('mail-body').value.trim();

            if (!subject) {
                alert('Please enter a subject');
                return;
            }

            try {
                const res = await fetch('/api/mail/send', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ to, subject, body })
                });
                const data = await res.json();

                if (data.success) {
                    alert('Mail sent to ' + to);
                    document.getElementById('mail-subject').value = '';
                    document.getElementById('mail-body').value = '';
                    loadAgentMail();
                } else {
                    alert('Failed: ' + (data.error || 'Unknown error'));
                }
            } catch (e) {
                alert('Error: ' + e.message);
            }
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // ========== Mayor Terminal Functions ==========
        let mayorTerminalSource = null;
        let mayorTerminalConnected = false;

        function toggleMayorTerminal() {
            if (mayorTerminalConnected) {
                disconnectMayorTerminal();
            } else {
                connectMayorTerminal();
            }
        }

        function connectMayorTerminal() {
            const output = document.getElementById('mayor-terminal');
            const toggle = document.getElementById('mayor-terminal-toggle');

            if (mayorTerminalSource) {
                mayorTerminalSource.close();
            }

            output.textContent = 'Connecting to hq-mayor...';
            mayorTerminalSource = new EventSource('/api/mayor/terminal');
            mayorTerminalConnected = true;
            toggle.textContent = 'Disconnect';
            toggle.style.background = '#f97316';

            mayorTerminalSource.addEventListener('frame', (event) => {
                output.textContent = event.data;
                output.scrollTop = output.scrollHeight;
            });

            mayorTerminalSource.addEventListener('error', (event) => {
                if (event.data) {
                    output.textContent = 'Error: ' + event.data;
                } else {
                    output.textContent = 'Mayor terminal disconnected. Click Connect to retry.';
                }
                disconnectMayorTerminal();
            });
        }

        function disconnectMayorTerminal() {
            const toggle = document.getElementById('mayor-terminal-toggle');
            if (mayorTerminalSource) {
                mayorTerminalSource.close();
                mayorTerminalSource = null;
            }
            mayorTerminalConnected = false;
            toggle.textContent = 'Connect';
            toggle.style.background = '#22c55e';
        }

        async function sendMailToMayor() {
            const subject = document.getElementById('mayor-mail-subject').value.trim();
            const body = document.getElementById('mayor-mail-body').value.trim();
            const result = document.getElementById('mayor-mail-result');

            if (!subject) {
                result.innerHTML = '<span style="color: #f87171;">Please enter a subject</span>';
                return;
            }

            result.innerHTML = '<span style="color: #fbbf24;">Sending...</span>';

            try {
                const res = await fetch('/api/mail/send', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ to: 'mayor/', subject, body })
                });
                const data = await res.json();

                if (data.success) {
                    result.innerHTML = '<span style="color: #4ade80;">✓ Mail sent to Mayor</span>';
                    document.getElementById('mayor-mail-subject').value = '';
                    document.getElementById('mayor-mail-body').value = '';
                    setTimeout(() => { result.innerHTML = ''; }, 3000);
                } else {
                    result.innerHTML = '<span style="color: #f87171;">Failed: ' + (data.error || 'Unknown error') + '</span>';
                }
            } catch (e) {
                result.innerHTML = '<span style="color: #f87171;">Error: ' + e.message + '</span>';
            }
        }

        async function updateMayorStatus() {
            try {
                const res = await fetch('/api/mayor/status');
                const data = await res.json();
                const indicator = document.getElementById('mayor-status');
                const hookSpan = document.getElementById('mayor-hook');

                if (data.session_exists) {
                    indicator.className = 'status-indicator status-green';
                } else {
                    indicator.className = 'status-indicator status-red';
                }

                if (data.hook && !data.hook.includes('Nothing on hook')) {
                    hookSpan.textContent = '🪝 Work hooked';
                    hookSpan.style.color = '#4ade80';
                } else {
                    hookSpan.textContent = 'No hook';
                    hookSpan.style.color = '#64748b';
                }
            } catch (e) {
                console.error('Failed to get mayor status:', e);
            }
        }

        // Update mayor status every 5 seconds
        setInterval(updateMayorStatus, 5000);

        // Initialize
        updateMayorStatus();
        loadAgentsList();

        // Start WebSocket updates
        connectStatusSocket();
    </script>
</body>
</html>`
