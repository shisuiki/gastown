package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// MailPageData is the data passed to the mail template.
type MailPageData struct {
	Title      string
	ActivePage string
}

// handleMail serves the mail center page.
func (h *GUIHandler) handleMail(w http.ResponseWriter, r *http.Request) {
	data := MailPageData{
		Title:      "Mail Center",
		ActivePage: "mail",
	}
	h.renderTemplate(w, "mail.html", data)
}

// handleAPISendMail handles sending mail.
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
	// Clear GT_ROLE so mail is sent from "overseer" (human via web), not mayor
	cmd.Env = filterEnv(os.Environ(), "GT_ROLE")
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

// handleAPIMailInbox gets the default inbox.
func (h *GUIHandler) handleAPIMailInbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check cache first
	if cached := h.cache.Get("mail:inbox"); cached != nil {
		w.Write(cached.([]byte))
		return
	}

	cmd := exec.Command("gt", "mail", "inbox", "--json")
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"messages": []interface{}{},
			"error":    err.Error(),
		})
		return
	}

	// Cache for 10 seconds
	h.cache.Set("mail:inbox", output, 10*time.Second)

	w.Write(output)
}

// handleAPIMailAll gets mail for any agent.
func (h *GUIHandler) handleAPIMailAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agent := r.URL.Query().Get("agent")
	if agent == "" {
		agent = "mayor/"
	}

	cacheKey := "mail:agent:" + agent

	// Check cache first
	if cached := h.cache.Get(cacheKey); cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// Get inbox for specific agent
	cmd := exec.Command("gt", "mail", "inbox", agent, "--json")
	output, err := cmd.Output()
	if err != nil {
		// Try without --json if it fails
		cmd2 := exec.Command("gt", "mail", "inbox", agent)
		output2, _ := cmd2.CombinedOutput()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agent": agent,
			"raw":   string(output2),
			"error": err.Error(),
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

	result := map[string]interface{}{
		"agent":    agent,
		"messages": messages,
	}

	// Cache for 10 seconds
	h.cache.Set(cacheKey, result, 10*time.Second)

	json.NewEncoder(w).Encode(result)
}

// handleAPIAgentsList returns all available agents for mail recipients.
func (h *GUIHandler) handleAPIAgentsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check cache first (agents list changes rarely)
	if cached := h.cache.Get("mail:agents"); cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

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

	// Cache for 60 seconds (agents list changes rarely)
	h.cache.Set("mail:agents", agents, 60*time.Second)

	json.NewEncoder(w).Encode(agents)
}

// filterEnv returns a copy of env with the specified key removed.
func filterEnv(env []string, key string) []string {
	result := make([]string, 0, len(env))
	prefix := key + "="
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}
