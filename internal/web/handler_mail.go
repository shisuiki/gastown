package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
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

type MailAgentSummary struct {
	Address    string `json:"address"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Unread     int    `json:"unread"`
	QueueCount int    `json:"queue_count"`
	HasQueue   bool   `json:"has_queue"`
}

type queueConfig struct {
	Name         string
	ClaimPattern string
}

type mailIndex struct {
	messagesByIdentity map[string][]*mail.Message
	unreadByIdentity   map[string]int
	queueMessages      map[string][]*mail.Message
	queueConfigs       []queueConfig
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

// handleAPIMailAgentView returns queue/inbox/archive data for a specific agent.
func (h *GUIHandler) handleAPIMailAgentView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agent := r.URL.Query().Get("agent")
	if agent == "" {
		agent = "mayor/"
	}

	limit := parseLimitParam(r, 200)
	result := h.fetchMailAgentView(agent, limit)
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

	// Use stale-while-revalidate (agents list + counts refresh frequently)
	cached := h.cache.GetStaleOrRefresh("mail_agents", 15*time.Second, func() interface{} {
		return h.fetchAgentsList()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchAgentsList()
	h.cache.Set("mail_agents", result, 15*time.Second)
	json.NewEncoder(w).Encode(result)
}

// fetchAgentsList gets available agents for mail.
func (h *GUIHandler) fetchAgentsList() []MailAgentSummary {
	agents := []MailAgentSummary{
		{Address: "overseer", Name: "Overseer", Type: "overseer"},
		{Address: "mayor/", Name: "Mayor", Type: "mayor"},
		{Address: "deacon/", Name: "Deacon", Type: "deacon"},
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
			agents = append(agents, MailAgentSummary{
				Address: name + "/",
				Name:    name,
				Type:    "crew",
			})
		}

		for _, polecat := range rigEntry.Polecats {
			name := rigEntry.Name + "/" + polecat
			agents = append(agents, MailAgentSummary{
				Address: name + "/",
				Name:    name,
				Type:    "polecat",
			})
		}

		if rigEntry.HasWitness {
			agents = append(agents, MailAgentSummary{
				Address: rigEntry.Name + "/witness/",
				Name:    rigEntry.Name + " Witness",
				Type:    "witness",
			})
		}
		if rigEntry.HasRefinery {
			agents = append(agents, MailAgentSummary{
				Address: rigEntry.Name + "/refinery/",
				Name:    rigEntry.Name + " Refinery",
				Type:    "refinery",
			})
		}
	}

	index, err := h.buildMailIndex()
	if err != nil {
		return agents
	}

	for i := range agents {
		identity := mailAddressToIdentity(agents[i].Address)
		agents[i].Unread = index.unreadByIdentity[identity]
		queueCount := index.queueCountForAgent(agents[i].Address)
		agents[i].QueueCount = queueCount
		agents[i].HasQueue = queueCount > 0
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

func (h *GUIHandler) fetchMailAgentView(agent string, limit int) map[string]interface{} {
	index, err := h.buildMailIndex()
	if err != nil {
		return map[string]interface{}{
			"agent":  agent,
			"error":  err.Error(),
			"inbox":  []*mail.Message{},
			"queue":  []*mail.Message{},
			"archive": []*mail.Message{},
		}
	}

	identity := mailAddressToIdentity(agent)
	inboxAll := append([]*mail.Message(nil), index.messagesByIdentity[identity]...)
	sort.Slice(inboxAll, func(i, j int) bool {
		return inboxAll[i].Timestamp.After(inboxAll[j].Timestamp)
	})
	inbox, inboxHasMore := applyMessageLimit(inboxAll, limit)

	queueAll := index.queueMessagesForAgent(agent)
	queue, queueHasMore := applyMessageLimit(queueAll, limit)

	archiveAll := []*mail.Message{}
	if mailbox, err := h.mailboxForAgent(agent); err == nil {
		if archived, err := mailbox.ListArchived(); err == nil {
			archiveAll = append(archiveAll, archived...)
		}
	}
	sort.Slice(archiveAll, func(i, j int) bool {
		return archiveAll[i].Timestamp.After(archiveAll[j].Timestamp)
	})
	archive, archiveHasMore := applyMessageLimit(archiveAll, limit)

	return map[string]interface{}{
		"agent":           agent,
		"inbox":           inbox,
		"inbox_total":     len(inboxAll),
		"inbox_unread":    index.unreadByIdentity[identity],
		"inbox_has_more":  inboxHasMore,
		"queue":           queue,
		"queue_total":     len(queueAll),
		"queue_has_more":  queueHasMore,
		"archive":         archive,
		"archive_total":   len(archiveAll),
		"archive_has_more": archiveHasMore,
	}
}

func applyMessageLimit(messages []*mail.Message, limit int) ([]*mail.Message, bool) {
	if limit <= 0 || len(messages) <= limit {
		return messages, false
	}
	return messages[:limit], true
}

func (h *GUIHandler) buildMailIndex() (*mailIndex, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, err
	}
	beadsDir := beads.ResolveBeadsDir(townRoot)
	reader, err := NewBeadsReaderWithBeadsDir(townRoot, beadsDir)
	if err != nil {
		return nil, err
	}

	messages, err := reader.ListBeads(BeadFilter{Type: "message", Limit: -1, IncludeEphemeral: true})
	if err != nil {
		return nil, err
	}

	index := &mailIndex{
		messagesByIdentity: make(map[string][]*mail.Message),
		unreadByIdentity:   make(map[string]int),
		queueMessages:      make(map[string][]*mail.Message),
	}

	queueBeads, err := reader.ListBeads(BeadFilter{Type: "queue", Limit: -1})
	if err == nil {
		for _, issue := range queueBeads {
			fields := beads.ParseQueueFields(issue.Description)
			name := strings.TrimSpace(fields.Name)
			if name == "" {
				name = strings.TrimPrefix(issue.ID, "hq-q-")
				name = strings.TrimPrefix(name, "gt-q-")
			}
			if name == "" {
				continue
			}
			index.queueConfigs = append(index.queueConfigs, queueConfig{
				Name:         name,
				ClaimPattern: fields.ClaimPattern,
			})
		}
	}

	seenByIdentity := make(map[string]map[string]bool)

	for _, bead := range messages {
		if bead.Status != "open" && bead.Status != "hooked" {
			continue
		}

		bm := mail.BeadsMessage{
			ID:          bead.ID,
			Title:       bead.Title,
			Description: bead.Description,
			Assignee:    bead.Assignee,
			Priority:    bead.Priority,
			Status:      bead.Status,
			CreatedAt:   bead.CreatedAt,
			Labels:      bead.Labels,
			Wisp:        bead.Ephemeral,
		}
		bm.ParseLabels()
		msg := bm.ToMessage()

		assignee := strings.TrimSpace(bm.Assignee)
		if assignee != "" && !strings.HasPrefix(assignee, "queue:") && !strings.HasPrefix(assignee, "channel:") && !strings.HasPrefix(assignee, "announce:") {
			identity := normalizeIdentity(assignee)
			addMessageToIndex(index, seenByIdentity, identity, msg)
		}

		for _, cc := range bm.GetCC() {
			identity := normalizeIdentity(cc)
			addMessageToIndex(index, seenByIdentity, identity, msg)
		}

		if bm.IsQueueMessage() && bm.GetClaimedBy() == "" {
			queueName := bm.GetQueue()
			if queueName != "" {
				index.queueMessages[queueName] = append(index.queueMessages[queueName], msg)
			}
		}
	}

	for identity, messages := range index.messagesByIdentity {
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].Timestamp.After(messages[j].Timestamp)
		})
		index.messagesByIdentity[identity] = messages
	}

	for queueName, messages := range index.queueMessages {
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].Timestamp.Before(messages[j].Timestamp)
		})
		index.queueMessages[queueName] = messages
	}

	return index, nil
}

func addMessageToIndex(index *mailIndex, seenByIdentity map[string]map[string]bool, identity string, msg *mail.Message) {
	if identity == "" || msg == nil || msg.ID == "" {
		return
	}
	if seenByIdentity[identity] == nil {
		seenByIdentity[identity] = make(map[string]bool)
	}
	if seenByIdentity[identity][msg.ID] {
		return
	}
	seenByIdentity[identity][msg.ID] = true
	index.messagesByIdentity[identity] = append(index.messagesByIdentity[identity], msg)
	if !msg.Read {
		index.unreadByIdentity[identity]++
	}
}

func normalizeIdentity(identity string) string {
	if identity == "mayor" {
		return "mayor/"
	}
	if identity == "deacon" {
		return "deacon/"
	}
	return identity
}

func mailAddressToIdentity(address string) string {
	if address == "overseer" {
		return "overseer"
	}
	if address == "mayor" || address == "mayor/" {
		return "mayor/"
	}
	if address == "deacon" || address == "deacon/" {
		return "deacon/"
	}
	if strings.HasSuffix(address, "/") {
		address = strings.TrimSuffix(address, "/")
	}
	parts := strings.Split(address, "/")
	if len(parts) == 3 && (parts[1] == "crew" || parts[1] == "polecats") {
		return parts[0] + "/" + parts[2]
	}
	return address
}

func addressForClaimPattern(address string) string {
	if address == "overseer" || address == "mayor/" || address == "deacon/" {
		return address
	}
	return strings.TrimSuffix(address, "/")
}

func (index *mailIndex) queueCountForAgent(agentAddr string) int {
	if index == nil {
		return 0
	}
	addr := addressForClaimPattern(agentAddr)
	count := 0
	for _, queue := range index.queueConfigs {
		if beads.MatchClaimPattern(queue.ClaimPattern, addr) {
			count += len(index.queueMessages[queue.Name])
		}
	}
	return count
}

func (index *mailIndex) queueMessagesForAgent(agentAddr string) []*mail.Message {
	if index == nil {
		return []*mail.Message{}
	}
	addr := addressForClaimPattern(agentAddr)
	seen := make(map[string]bool)
	var messages []*mail.Message
	for _, queue := range index.queueConfigs {
		if !beads.MatchClaimPattern(queue.ClaimPattern, addr) {
			continue
		}
		for _, msg := range index.queueMessages[queue.Name] {
			if msg == nil || msg.ID == "" || seen[msg.ID] {
				continue
			}
			seen[msg.ID] = true
			messages = append(messages, msg)
		}
	}
	if len(messages) == 0 {
		return []*mail.Message{}
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})
	return messages
}
