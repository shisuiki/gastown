package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/workspace"
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

	router, err := h.mailRouter()
	if err != nil {
		http.Error(w, "Mail router error", http.StatusInternalServerError)
		return
	}

	msg := &mail.Message{
		From:     "overseer",
		To:       req.To,
		Subject:  req.Subject,
		Body:     req.Body,
		Priority: mail.PriorityNormal,
		Type:     mail.TypeNotification,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := router.Send(msg); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
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
	router, err := h.mailRouter()
	if err != nil {
		return map[string]interface{}{
			"messages": []*mail.Message{},
			"error":    err.Error(),
		}
	}

	mailbox, err := router.GetMailbox("mayor/")
	if err != nil {
		return map[string]interface{}{
			"messages": []*mail.Message{},
			"error":    err.Error(),
		}
	}

	messages, err := mailbox.List()
	if err != nil {
		return map[string]interface{}{
			"messages": []*mail.Message{},
			"error":    err.Error(),
		}
	}

	return buildMailResponse("mayor/", messages, limit)
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
	router, err := h.mailRouter()
	if err != nil {
		return map[string]interface{}{
			"agent":    agent,
			"messages": []*mail.Message{},
			"error":    err.Error(),
		}
	}

	mailbox, err := router.GetMailbox(agent)
	if err != nil {
		return map[string]interface{}{
			"agent":    agent,
			"messages": []*mail.Message{},
			"error":    err.Error(),
		}
	}

	messages, err := mailbox.List()
	if err != nil {
		return map[string]interface{}{
			"agent":    agent,
			"messages": []*mail.Message{},
			"error":    err.Error(),
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

func buildMailResponse(agent string, messages []*mail.Message, limit int) map[string]interface{} {
	total := len(messages)
	unread := 0
	for _, msg := range messages {
		if msg != nil && !msg.Read {
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

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return agents
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return agents
	}

	manager := rig.NewManager(townRoot, rigsConfig, git.NewGit(townRoot))
	rigs, err := manager.DiscoverRigs()
	if err != nil {
		return agents
	}

	for _, rigEntry := range rigs {
		for _, crew := range rigEntry.Crew {
			name := rigEntry.Name + "/" + crew
			agents = append(agents, map[string]string{
				"address": name + "/",
				"name":    name,
				"type":    "crew",
			})
		}

		for _, polecat := range rigEntry.Polecats {
			name := rigEntry.Name + "/" + polecat
			agents = append(agents, map[string]string{
				"address": name + "/",
				"name":    name,
				"type":    "polecat",
			})
		}

		if rigEntry.HasWitness {
			agents = append(agents, map[string]string{
				"address": rigEntry.Name + "/witness/",
				"name":    rigEntry.Name + " Witness",
				"type":    "witness",
			})
		}
		if rigEntry.HasRefinery {
			agents = append(agents, map[string]string{
				"address": rigEntry.Name + "/refinery/",
				"name":    rigEntry.Name + " Refinery",
				"type":    "refinery",
			})
		}
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

	w.Header().Set("Content-Type", "application/json")
	mailbox, err := h.mailboxForAgent(agent)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := mailbox.MarkReadOnly(req.ID); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
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

	w.Header().Set("Content-Type", "application/json")
	mailbox, err := h.mailboxForAgent(agent)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := mailbox.MarkUnreadOnly(req.ID); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
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

	w.Header().Set("Content-Type", "application/json")
	mailbox, err := h.mailboxForAgent(agent)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := mailbox.Archive(req.ID); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (h *GUIHandler) mailRouter() (*mail.Router, error) {
	townRoot := os.Getenv("GT_ROOT")
	if townRoot == "" {
		root, err := workspace.FindFromCwdOrError()
		if err != nil {
			return nil, err
		}
		townRoot = root
	}
	return mail.NewRouterWithTownRoot(townRoot, townRoot), nil
}

func (h *GUIHandler) mailboxForAgent(agent string) (*mail.Mailbox, error) {
	router, err := h.mailRouter()
	if err != nil {
		return nil, err
	}
	return router.GetMailbox(agent)
}
