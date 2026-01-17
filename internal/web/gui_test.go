package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func setupFakeGT(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "gt")
	script := `#!/bin/sh
cmd="$1"
shift
case "$cmd" in
  daemon)
    if [ "$1" = "status" ]; then
      echo "daemon running"
      exit 0
    fi
    ;;
  rig)
    if [ "$1" = "list" ]; then
      echo "Rigs"
      echo "rig-alpha"
      exit 0
    fi
    ;;
  mail)
    if [ "$1" = "inbox" ]; then
      echo "3 unread"
      exit 0
    fi
    if [ "$1" = "send" ]; then
      echo "sent"
      exit 0
    fi
    ;;
  status)
    echo "ok"
    exit 0
    ;;
esac
echo "unknown command" >&2
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	path := dir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)
}

func newGUIHandler(t *testing.T) *GUIHandler {
	t.Helper()
	setupFakeGT(t)

	mock := &MockConvoyFetcher{
		Convoys: []ConvoyRow{
			{ID: "hq-cv-123", Title: "Convoy", Status: "open"},
		},
		MergeQueue: []MergeQueueRow{
			{Repo: "repo", Title: "PR"},
		},
		Agents: []AgentRow{
			{Name: "agent-1"},
		},
	}

	handler, err := NewGUIHandler(mock)
	if err != nil {
		t.Fatalf("NewGUIHandler() error = %v", err)
	}
	return handler
}

func TestGUIHandler_APIStatus(t *testing.T) {
	handler := newGUIHandler(t)

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("Decode error = %v", err)
	}

	if len(status.Convoys) != 1 || status.Convoys[0].ID != "hq-cv-123" {
		t.Errorf("Convoys = %#v, want mock data", status.Convoys)
	}
	if len(status.MergeQueue) != 1 {
		t.Errorf("MergeQueue length = %d, want 1", len(status.MergeQueue))
	}
	if len(status.Agents) != 1 {
		t.Errorf("Agents length = %d, want 1", len(status.Agents))
	}
	if status.Daemon.Running != true {
		t.Errorf("Daemon running = %v, want true", status.Daemon.Running)
	}
}

func TestGUIHandler_APISendMail(t *testing.T) {
	handler := newGUIHandler(t)

	server := httptest.NewServer(handler)
	defer server.Close()

	payload := []byte(`{"to":"test@example.com","subject":"Hello","body":"Hi"}`)
	resp, err := http.Post(server.URL+"/api/mail/send", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/mail/send error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("Decode error = %v", err)
	}

	if out["success"] != true {
		t.Fatalf("success = %v, want true", out["success"])
	}
	if !strings.Contains(out["output"].(string), "sent") {
		t.Fatalf("output = %q, want contains \"sent\"", out["output"])
	}
}

func TestGUIHandler_APICommand(t *testing.T) {
	handler := newGUIHandler(t)

	server := httptest.NewServer(handler)
	defer server.Close()

	payload := []byte(`{"command":"status","args":["--json"]}`)
	resp, err := http.Post(server.URL+"/api/command", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/command error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("Decode error = %v", err)
	}

	if out["success"] != true {
		t.Fatalf("success = %v, want true", out["success"])
	}
	if !strings.Contains(out["output"].(string), "ok") {
		t.Fatalf("output = %q, want contains \"ok\"", out["output"])
	}
}

func TestGUIHandler_StatusWebSocket(t *testing.T) {
	handler := newGUIHandler(t)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/status"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline error = %v", err)
	}

	var status StatusResponse
	if err := conn.ReadJSON(&status); err != nil {
		t.Fatalf("ReadJSON error = %v", err)
	}

	if len(status.Convoys) != 1 || status.Convoys[0].ID != "hq-cv-123" {
		t.Errorf("Convoys = %#v, want mock data", status.Convoys)
	}
}

func TestNewGUIHandler_RejectsInsecureRemoteConfig(t *testing.T) {
	// Save original values
	origToken := authConfig.token
	origAllowRemote := authConfig.allowRemote
	defer func() {
		authConfig.token = origToken
		authConfig.allowRemote = origAllowRemote
	}()

	// Test: allowRemote=true without token should fail
	authConfig.token = ""
	authConfig.allowRemote = true

	mock := &MockConvoyFetcher{}
	_, err := NewGUIHandler(mock)

	if err == nil {
		t.Error("NewGUIHandler() should reject insecure remote config (allowRemote=true, token=empty)")
	}
	if err != ErrInsecureRemoteConfig {
		t.Errorf("NewGUIHandler() error = %v, want ErrInsecureRemoteConfig", err)
	}

	// Test: allowRemote=true with token should succeed
	authConfig.token = "test-token"
	authConfig.allowRemote = true

	_, err = NewGUIHandler(mock)
	if err != nil {
		t.Errorf("NewGUIHandler() with token should succeed, got error = %v", err)
	}

	// Test: allowRemote=false without token should succeed (localhost only)
	authConfig.token = ""
	authConfig.allowRemote = false

	_, err = NewGUIHandler(mock)
	if err != nil {
		t.Errorf("NewGUIHandler() localhost-only should succeed, got error = %v", err)
	}
}
