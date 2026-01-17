package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
)

// ActivityPageData is the data passed to the activity template.
type ActivityPageData struct {
	Title      string
	ActivePage string
}

// handleActivity serves the activity page.
func (h *GUIHandler) handleActivity(w http.ResponseWriter, r *http.Request) {
	data := ActivityPageData{
		Title:      "Activity",
		ActivePage: "activity",
	}
	h.renderTemplate(w, "activity.html", data)
}

// handleAPIActivity returns recent git commits and activity.
func (h *GUIHandler) handleAPIActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get recent commits from gastown-src
	cmd := exec.Command("git", "log", "--oneline", "-20", "--format=%h|%s|%cr|%an")
	cmd.Dir = "/home/shisui/gt/gastown-src"
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"commits": []interface{}{},
			"error":   err.Error(),
		})
		return
	}

	var commits []map[string]string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			commits = append(commits, map[string]string{
				"hash":    parts[0],
				"message": parts[1],
				"age":     parts[2],
				"author":  parts[3],
			})
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"commits": commits,
	})
}
