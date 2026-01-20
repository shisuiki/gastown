package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

func TestCrewList_Empty(t *testing.T) {
	root := setupCrewTown(t, "terra", nil)
	cwd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("GT_CACHE_DIR", t.TempDir())

	handler, err := NewGUIHandler(&MockConvoyFetcher{})
	if err != nil {
		t.Fatalf("NewGUIHandler: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/crew/list?rig=terra", nil)
	w := httptest.NewRecorder()

	handler.handleAPICrewList(w, req)

	var resp CrewListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Count != 0 {
		t.Fatalf("expected 0 crew, got %d", resp.Count)
	}
}

func TestCrewList_WithCrew(t *testing.T) {
	root := setupCrewTown(t, "terra", []string{"alice"})
	cwd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("GT_CACHE_DIR", t.TempDir())

	handler, err := NewGUIHandler(&MockConvoyFetcher{})
	if err != nil {
		t.Fatalf("NewGUIHandler: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/crew/list?rig=terra", nil)
	w := httptest.NewRecorder()

	handler.handleAPICrewList(w, req)

	var resp CrewListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Count != 1 {
		t.Fatalf("expected 1 crew, got %d", resp.Count)
	}
	if len(resp.Crew) != 1 || resp.Crew[0].Name != "alice" {
		t.Fatalf("unexpected crew list: %+v", resp.Crew)
	}
	if resp.Crew[0].Rig != "terra" {
		t.Fatalf("expected rig terra, got %s", resp.Crew[0].Rig)
	}
}

func setupCrewTown(t *testing.T, rigName string, crewNames []string) string {
	t.Helper()
	root := t.TempDir()

	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	town := config.TownConfig{
		Type:      "town",
		Version:   config.CurrentTownVersion,
		Name:      "test-town",
		CreatedAt: time.Now(),
	}
	writeJSON(t, filepath.Join(mayorDir, "town.json"), town)

	rigs := config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			rigName: {
				GitURL:  "https://example.com/" + rigName + ".git",
				AddedAt: time.Now(),
			},
		},
	}
	writeJSON(t, filepath.Join(mayorDir, "rigs.json"), rigs)

	rigDir := filepath.Join(root, rigName, "crew")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rig crew: %v", err)
	}
	for _, name := range crewNames {
		if err := os.MkdirAll(filepath.Join(rigDir, name), 0755); err != nil {
			t.Fatalf("mkdir crew %s: %v", name, err)
		}
	}

	return root
}

func writeJSON(t *testing.T, path string, data interface{}) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
