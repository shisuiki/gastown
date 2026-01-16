package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"
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
	h.mux.HandleFunc("/api/mail/send", h.handleAPISendMail)
	h.mux.HandleFunc("/api/mail/inbox", h.handleAPIMailInbox)
	h.mux.HandleFunc("/api/command", h.handleAPICommand)
	h.mux.HandleFunc("/api/rigs", h.handleAPIRigs)
	h.mux.HandleFunc("/api/convoys", h.handleAPIConvoys)

	return h, nil
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

	json.NewEncoder(w).Encode(status)
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
	var rigs []RigStatus
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Rigs") {
			continue
		}
		// Parse: "  rigname\n    Polecats: N  Crew: M"
		if !strings.HasPrefix(line, " ") && line != "" {
			rigs = append(rigs, RigStatus{Name: line})
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
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Gas Town <span class="status-indicator status-green" id="daemon-status"></span></h1>
            <span id="status-time">Loading...</span>
        </header>

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

            <div class="card">
                <h2>📬 Mail</h2>
                <div class="card-content" id="mail-status">Loading...</div>
            </div>

            <div class="card chat-container">
                <h2>💬 Talk to Mayor</h2>
                <div class="chat-messages" id="chat-messages"></div>
                <div class="chat-input">
                    <input type="text" id="chat-input" placeholder="Send a message to Mayor..." />
                    <button onclick="sendMessage()">Send</button>
                </div>
            </div>
        </div>
    </div>

    <script>
        async function fetchStatus() {
            try {
                const res = await fetch('/api/status');
                const data = await res.json();
                updateUI(data);
            } catch (e) {
                console.error('Failed to fetch status:', e);
            }
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

        // Handle Enter key in chat input
        document.getElementById('chat-input').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') sendMessage();
        });

        // Initial fetch and auto-refresh every 30 seconds
        fetchStatus();
        setInterval(fetchStatus, 30000);
    </script>
</body>
</html>`
