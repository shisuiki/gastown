package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// TerminalsPageData is the data passed to the terminals template.
type TerminalsPageData struct {
	Title      string
	ActivePage string
}

// handleTerminals serves the terminals page.
func (h *GUIHandler) handleTerminals(w http.ResponseWriter, r *http.Request) {
	data := TerminalsPageData{
		Title:      "Terminals",
		ActivePage: "terminals",
	}
	h.renderTemplate(w, "terminals.html", data)
}

// tmuxSessionNamePattern matches polecat sessions (gt-rig-name) and HQ sessions (hq-mayor, hq-deacon)
var tmuxSessionNamePattern = regexp.MustCompile(`^(gt-[a-zA-Z0-9_-]+-[a-zA-Z0-9_-]+|hq-(mayor|deacon))$`)
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// handleAPITerminalStream streams a polecat terminal session.
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
	lastPing := time.Now()
	errorCount := 0
	const maxConsecutiveErrors = 5
	const keepaliveInterval = 10 * time.Second // Reduced from 15s to prevent proxy/mobile timeout

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			frame, err := captureTmuxPane(session)
			if err != nil {
				errorCount++
				// Distinguish between transient errors and session-ended
				errMsg := err.Error()
				if strings.Contains(errMsg, "no server running") ||
					strings.Contains(errMsg, "session not found") ||
					strings.Contains(errMsg, "can't find") {
					// Session ended - notify client and close
					writeSSE(w, "error", "session_ended:"+errMsg)
					flusher.Flush()
					return
				}

				// Transient error - send error event but keep stream alive
				writeSSE(w, "error", "transient:"+errMsg)
				flusher.Flush()

				// Give up after too many consecutive errors
				if errorCount >= maxConsecutiveErrors {
					writeSSE(w, "error", "max_errors_reached")
					flusher.Flush()
					return
				}
				continue
			}

			// Reset error count on successful capture
			errorCount = 0

			frame = strings.TrimRight(frame, "\n")
			if frame == lastFrame {
				// Send keepalive based on time elapsed (more reliable than count)
				if time.Since(lastPing) >= keepaliveInterval {
					writeSSE(w, "ping", "keepalive")
					flusher.Flush()
					lastPing = time.Now()
				}
				continue
			}
			lastPing = time.Now() // Reset ping timer on actual data
			lastFrame = frame
			writeSSE(w, "frame", frame)
			flusher.Flush()
		}
	}
}

// captureTmuxPane captures the content of a tmux pane.
func captureTmuxPane(session string) (string, error) {
	cmd, cancel := command("tmux", "capture-pane", "-t", session, "-p", "-J", "-S", "-2000")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return sanitizeTerminalOutput(string(output)), nil
}

// sanitizeTerminalOutput removes ANSI escape codes and control characters.
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

// writeSSE writes a Server-Sent Event to the response.
func writeSSE(w http.ResponseWriter, event, data string) {
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

// TerminalSendRequest represents a request to send input to a terminal.
type TerminalSendRequest struct {
	Session string `json:"session"`
	Text    string `json:"text,omitempty"`
	Key     string `json:"key,omitempty"`
	Enter   bool   `json:"enter,omitempty"`
}

// handleAPITerminalSend sends input to a tmux session.
func (h *GUIHandler) handleAPITerminalSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TerminalSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Session == "" || !tmuxSessionNamePattern.MatchString(req.Session) {
		http.Error(w, "Invalid session", http.StatusBadRequest)
		return
	}

	// Send key if specified (for special keys like C-c, Enter)
	if req.Key != "" {
		if err := sendTmuxKey(req.Session, req.Key); err != nil {
			http.Error(w, "Failed to send key: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// Send text if specified
	if req.Text != "" {
		if err := sendTmuxText(req.Session, req.Text, req.Enter); err != nil {
			http.Error(w, "Failed to send text: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// sendTmuxKey sends a special key to a tmux session.
func sendTmuxKey(session, key string) error {
	cmd, cancel := command("tmux", "send-keys", "-t", session, key)
	defer cancel()
	return cmd.Run()
}

// sendTmuxText sends text to a tmux session.
func sendTmuxText(session, text string, enter bool) error {
	if enter {
		text += "\r"
	}
	// Use send-keys with literal text (including optional newline)
	args := []string{"send-keys", "-t", session, "-l", text}
	cmd, cancel := command("tmux", args...)
	defer cancel()
	return cmd.Run()
}
