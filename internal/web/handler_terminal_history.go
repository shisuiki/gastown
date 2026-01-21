package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/constants"
)

const (
	terminalHistoryFileName        = "terminal_history.json"
	terminalHistoryDefaultPageSize = 25
	terminalHistoryMaxEntries      = 500
)

var terminalHistoryKeys = map[string]struct{}{
	"terminal-history": {},
	"mayor-history":    {},
}

type terminalHistoryEntry struct {
	Timestamp int64  `json:"timestamp"`
	Time      string `json:"time,omitempty"`
	Target    string `json:"target"`
	Context   string `json:"context"`
}

type terminalHistoryStore struct {
	Entries  []terminalHistoryEntry `json:"entries"`
	PageSize int                    `json:"page_size"`
}

type terminalHistoryFile struct {
	Histories map[string]terminalHistoryStore `json:"histories"`
}

type terminalHistoryRequest struct {
	StorageKey string                `json:"storage_key"`
	Entry      *terminalHistoryEntry `json:"entry,omitempty"`
	PageSize   int                   `json:"page_size,omitempty"`
}

func (h *GUIHandler) handleAPITerminalHistory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleAPITerminalHistoryGet(w, r)
	case http.MethodPost:
		h.handleAPITerminalHistoryPost(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *GUIHandler) handleAPITerminalHistoryGet(w http.ResponseWriter, r *http.Request) {
	key, ok := parseTerminalHistoryKey(r.URL.Query().Get("key"))
	if !ok {
		http.Error(w, "Invalid history key", http.StatusBadRequest)
		return
	}

	h.historyMu.Lock()
	defer h.historyMu.Unlock()

	_, path := terminalHistoryPath()
	payload, err := readTerminalHistory(path)
	if err != nil {
		http.Error(w, "Failed to load history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	store := payload.Histories[key]
	if store.PageSize <= 0 {
		store.PageSize = terminalHistoryDefaultPageSize
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(store)
}

func (h *GUIHandler) handleAPITerminalHistoryPost(w http.ResponseWriter, r *http.Request) {
	var req terminalHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	key, ok := parseTerminalHistoryKey(req.StorageKey)
	if !ok {
		http.Error(w, "Invalid history key", http.StatusBadRequest)
		return
	}

	h.historyMu.Lock()
	defer h.historyMu.Unlock()

	runtimeDir, path := terminalHistoryPath()
	payload, err := readTerminalHistory(path)
	if err != nil {
		http.Error(w, "Failed to load history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	store := payload.Histories[key]
	if store.PageSize <= 0 {
		store.PageSize = terminalHistoryDefaultPageSize
	}

	if req.PageSize > 0 {
		store.PageSize = req.PageSize
	}

	if req.Entry != nil {
		record := normalizeTerminalHistoryEntry(*req.Entry)
		store.Entries = append([]terminalHistoryEntry{record}, store.Entries...)
		if len(store.Entries) > terminalHistoryMaxEntries {
			store.Entries = store.Entries[:terminalHistoryMaxEntries]
		}
	}

	if payload.Histories == nil {
		payload.Histories = make(map[string]terminalHistoryStore)
	}
	payload.Histories[key] = store

	if err := writeTerminalHistory(runtimeDir, path, payload); err != nil {
		http.Error(w, "Failed to save history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(store)
}

func parseTerminalHistoryKey(key string) (string, bool) {
	key = strings.TrimSpace(key)
	if _, ok := terminalHistoryKeys[key]; ok {
		return key, true
	}
	return "", false
}

func normalizeTerminalHistoryEntry(entry terminalHistoryEntry) terminalHistoryEntry {
	if entry.Timestamp <= 0 {
		entry.Timestamp = time.Now().UnixMilli()
	}
	entry.Target = strings.TrimSpace(entry.Target)
	if entry.Target == "" {
		entry.Target = "unknown"
	}
	entry.Context = strings.TrimSpace(entry.Context)
	if entry.Time == "" {
		entry.Time = time.UnixMilli(entry.Timestamp).UTC().Format(time.RFC3339)
	}
	return entry
}

func terminalHistoryPath() (string, string) {
	workDir := webWorkDir()
	runtimeDir := filepath.Join(workDir, constants.DirRuntime)
	return runtimeDir, filepath.Join(runtimeDir, terminalHistoryFileName)
}

func readTerminalHistory(path string) (terminalHistoryFile, error) {
	payload := terminalHistoryFile{Histories: make(map[string]terminalHistoryStore)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return payload, nil
		}
		return terminalHistoryFile{}, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return terminalHistoryFile{}, err
	}
	if payload.Histories == nil {
		payload.Histories = make(map[string]terminalHistoryStore)
	}
	return payload, nil
}

func writeTerminalHistory(runtimeDir, path string, payload terminalHistoryFile) error {
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(runtimeDir, "terminal_history_*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpFile.Name(), 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile.Name(), path)
}
