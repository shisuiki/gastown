package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// MayorPageData is the data passed to the mayor template.
type MayorPageData struct {
	Title      string
	ActivePage string
}

// handleMayor serves the mayor control page.
func (h *GUIHandler) handleMayor(w http.ResponseWriter, r *http.Request) {
	data := MayorPageData{
		Title:      "Mayor Control",
		ActivePage: "mayor",
	}
	h.renderTemplate(w, "mayor.html", data)
}

// handleAPIMayorTerminal streams the mayor's tmux session output.
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
	errorCount := 0
	const maxConsecutiveErrors = 5

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			frame, err := captureTmuxPane("hq-mayor")
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

// handleAPIMayorStatus returns mayor session status.
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
