package web

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/mail"
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

	cmd, cancel := command("gt", args...)
	defer cancel()
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

	limit := parseLimitParam(r, 200)
	cacheKey := "mail_inbox_" + strconv.Itoa(limit)

	// Use stale-while-revalidate
	cached := h.cache.GetStaleOrRefresh(cacheKey, 10*time.Second, func() interface{} {
		return h.fetchMailInbox(limit)
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchMailInbox(limit)
	h.cache.Set(cacheKey, result, 10*time.Second)
	json.NewEncoder(w).Encode(result)
}

// fetchMailInbox gets mail inbox data.
func (h *GUIHandler) fetchMailInbox(limit int) map[string]interface{} {
	cmd, cancel := command("gt", "mail", "inbox", "--json")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{
			"messages": []mail.Message{},
			"error":    err.Error(),
		}
	}

	var messages []mail.Message
	if err := json.Unmarshal(output, &messages); err != nil {
		return map[string]interface{}{
			"messages": []mail.Message{},
			"error":    "parse error",
		}
	}
	return buildMailResponse("", messages, limit)
}

// handleAPIMailAll gets mail for any agent.
func (h *GUIHandler) handleAPIMailAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agent := r.URL.Query().Get("agent")
	if agent == "" {
		agent = "mayor/"
	}

	limit := parseLimitParam(r, 200)
	cacheKey := "mail_agent_" + sanitizeAgentKey(agent) + "_" + strconv.Itoa(limit)

	// Use stale-while-revalidate
	cached := h.cache.GetStaleOrRefresh(cacheKey, 10*time.Second, func() interface{} {
		return h.fetchMailForAgent(agent, limit)
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchMailForAgent(agent, limit)
	h.cache.Set(cacheKey, result, 10*time.Second)
	json.NewEncoder(w).Encode(result)
}

// sanitizeAgentKey converts agent address to safe cache key.
func sanitizeAgentKey(agent string) string {
	safe := ""
	for _, r := range agent {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			safe += string(r)
		} else {
			safe += "_"
		}
	}
	return safe
}

// fetchMailForAgent gets mail for a specific agent.
func (h *GUIHandler) fetchMailForAgent(agent string, limit int) map[string]interface{} {
	cmd, cancel := command("gt", "mail", "inbox", agent, "--json")
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		// Try without --json if it fails
		cmd2, cmdCancel := command("gt", "mail", "inbox", agent)
		defer cmdCancel()
		output2, _ := cmd2.CombinedOutput()
		return map[string]interface{}{
			"agent": agent,
			"raw":   string(output2),
			"error": err.Error(),
		}
	}

	// Parse and forward the JSON
	var messages []mail.Message
	if err := json.Unmarshal(output, &messages); err != nil {
		return map[string]interface{}{
			"agent": agent,
			"raw":   string(output),
		}
	}

	return buildMailResponse(agent, messages, limit)
}

func parseLimitParam(r *http.Request, defaultLimit int) int {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return defaultLimit
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 0 {
		return defaultLimit
	}
	return limit
}

func buildMailResponse(agent string, messages []mail.Message, limit int) map[string]interface{} {
	total := len(messages)
	unread := 0
	for _, msg := range messages {
		if !msg.Read {
			unread++
		}
	}

	hasMore := false
	if limit > 0 && total > limit {
		messages = messages[:limit]
		hasMore = true
	}

	resp := map[string]interface{}{
		"messages": messages,
		"total":    total,
		"unread":   unread,
		"limit":    limit,
		"has_more": hasMore,
	}
	if agent != "" {
		resp["agent"] = agent
	}
	return resp
}

// handleAPIAgentsList returns all available agents for mail recipients.
func (h *GUIHandler) handleAPIAgentsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Use stale-while-revalidate (agents list changes rarely, 60s TTL)
	cached := h.cache.GetStaleOrRefresh("mail_agents", 60*time.Second, func() interface{} {
		return h.fetchAgentsList()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchAgentsList()
	h.cache.Set("mail_agents", result, 60*time.Second)
	json.NewEncoder(w).Encode(result)
}

// fetchAgentsList gets available agents for mail.
func (h *GUIHandler) fetchAgentsList() []map[string]string {
	agents := []map[string]string{
		{"address": "overseer", "name": "Overseer", "type": "overseer"},
		{"address": "mayor/", "name": "Mayor", "type": "mayor"},
		{"address": "deacon/", "name": "Deacon", "type": "deacon"},
	}

	// Get crew from all rigs
	cmd, cancel := command("gt", "crew", "list", "--all")
	defer cancel()
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
	cmd, cancel = command("gt", "polecat", "list", "--all")
	defer cancel()
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

	return agents
}

// handleAPIMailMarkRead marks a message as read.
func (h *GUIHandler) handleAPIMailMarkRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID    string `json:"id"`
		Agent string `json:"agent,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "Missing message ID", http.StatusBadRequest)
		return
	}

	agent := req.Agent
	if agent == "" {
		agent = "mayor/"
	}

	// Set GT_ROLE environment variable to target agent
	env := os.Environ()
	// Remove existing GT_ROLE
	env = filterEnv(env, "GT_ROLE")
	// Add new GT_ROLE
	env = append(env, "GT_ROLE="+agent)

	cmd, cancel := command("gt", "mail", "mark-read", req.ID)
	defer cancel()
	cmd.Env = env
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

// handleAPIMailMarkUnread marks a message as unread.
func (h *GUIHandler) handleAPIMailMarkUnread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID    string `json:"id"`
		Agent string `json:"agent,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "Missing message ID", http.StatusBadRequest)
		return
	}

	agent := req.Agent
	if agent == "" {
		agent = "mayor/"
	}

	// Set GT_ROLE environment variable to target agent
	env := os.Environ()
	// Remove existing GT_ROLE
	env = filterEnv(env, "GT_ROLE")
	// Add new GT_ROLE
	env = append(env, "GT_ROLE="+agent)

	cmd, cancel := command("gt", "mail", "mark-unread", req.ID)
	defer cancel()
	cmd.Env = env
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

// handleAPIMailArchive archives a message.
func (h *GUIHandler) handleAPIMailArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID    string `json:"id"`
		Agent string `json:"agent,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "Missing message ID", http.StatusBadRequest)
		return
	}

	agent := req.Agent
	if agent == "" {
		agent = "mayor/"
	}

	// Set GT_ROLE environment variable to target agent
	env := os.Environ()
	// Remove existing GT_ROLE
	env = filterEnv(env, "GT_ROLE")
	// Add new GT_ROLE
	env = append(env, "GT_ROLE="+agent)

	cmd, cancel := command("gt", "mail", "archive", req.ID)
	defer cancel()
	cmd.Env = env
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
