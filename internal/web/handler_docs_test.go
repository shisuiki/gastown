package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocsRepoRoot_UsesEnvRepoRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	t.Setenv("GASTOWN_DOCS_ROOT", root)
	t.Setenv("GASTOWN_SRC", "")
	t.Setenv("GT_ROOT", "")

	got, err := docsRepoRoot()
	if err != nil {
		t.Fatalf("docsRepoRoot() error = %v", err)
	}
	if got != root {
		t.Fatalf("docsRepoRoot() = %q, want %q", got, root)
	}
}

func TestDocsRepoRoot_UsesEnvDocsDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	t.Setenv("GASTOWN_DOCS_ROOT", "")
	t.Setenv("GASTOWN_SRC", root)
	t.Setenv("GT_ROOT", "")

	got, err := docsRepoRoot()
	if err != nil {
		t.Fatalf("docsRepoRoot() error = %v", err)
	}
	if got != root {
		t.Fatalf("docsRepoRoot() = %q, want %q", got, root)
	}
}
