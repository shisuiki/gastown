package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeIssuesJSONL(t *testing.T, dir string, lines []string) {
	t.Helper()
	path := filepath.Join(dir, "issues.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestReadyBeadsPriorityOrder(t *testing.T) {
	beadsDir := t.TempDir()
	writeIssuesJSONL(t, beadsDir, []string{
		`{"id":"hq-a","title":"P0","status":"open","priority":0,"issue_type":"task","created_at":"2026-01-01T00:00:00Z"}`,
		`{"id":"hq-b","title":"P2","status":"open","priority":2,"issue_type":"task","created_at":"2026-01-02T00:00:00Z"}`,
	})

	reader, err := NewBeadsReaderWithBeadsDir("", beadsDir)
	if err != nil {
		t.Fatalf("NewBeadsReaderWithBeadsDir() error = %v", err)
	}

	ready, err := reader.ReadyBeads()
	if err != nil {
		t.Fatalf("ReadyBeads() error = %v", err)
	}
	if len(ready) != 2 {
		t.Fatalf("ReadyBeads() len = %d, want 2", len(ready))
	}
	if ready[0].ID != "hq-a" {
		t.Fatalf("ReadyBeads()[0].ID = %s, want hq-a", ready[0].ID)
	}
}

func TestReadyBeadsDeferredFiltering(t *testing.T) {
	beadsDir := t.TempDir()
	now := time.Now().UTC()
	future := now.Add(1 * time.Hour).Format(time.RFC3339)
	past := now.Add(-1 * time.Hour).Format(time.RFC3339)

	writeIssuesJSONL(t, beadsDir, []string{
		`{"id":"hq-defer-future","title":"Deferred","status":"open","priority":1,"issue_type":"task","defer_until":"` + future + `"}`,
		`{"id":"hq-defer-past","title":"Ready","status":"open","priority":1,"issue_type":"task","defer_until":"` + past + `"}`,
		`{"id":"hq-ready","title":"Ready","status":"open","priority":2,"issue_type":"task"}`,
	})

	reader, err := NewBeadsReaderWithBeadsDir("", beadsDir)
	if err != nil {
		t.Fatalf("NewBeadsReaderWithBeadsDir() error = %v", err)
	}

	ready, err := reader.ReadyBeads()
	if err != nil {
		t.Fatalf("ReadyBeads() error = %v", err)
	}

	ids := make(map[string]bool)
	for _, bead := range ready {
		ids[bead.ID] = true
	}

	if ids["hq-defer-future"] {
		t.Fatalf("Deferred future bead should be filtered")
	}
	if !ids["hq-defer-past"] {
		t.Fatalf("Deferred past bead should be ready")
	}
	if !ids["hq-ready"] {
		t.Fatalf("Non-deferred bead should be ready")
	}
}
