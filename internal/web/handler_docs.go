package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/workspace"
)

const maxDocBytes = 2 * 1024 * 1024

type docsTreeResponse struct {
	Files []string `json:"files"`
}

// handleDocs serves the documentation browser page.
func (h *GUIHandler) handleDocs(w http.ResponseWriter, r *http.Request) {
	data := struct {
		ActivePage string
	}{
		ActivePage: "docs",
	}
	h.renderTemplate(w, "docs.html", data)
}

// handleAPIDocsTree returns a list of markdown files under the repo root.
func (h *GUIHandler) handleAPIDocsTree(w http.ResponseWriter, r *http.Request) {
	repoRoot, err := docsRepoRoot()
	if err != nil {
		http.Error(w, "Failed to locate repo root", http.StatusInternalServerError)
		return
	}

	files, err := collectMarkdownFiles(repoRoot)
	if err != nil {
		http.Error(w, "Failed to list docs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docsTreeResponse{Files: files})
}

// handleAPIDocsFile returns the markdown content for a given path.
func (h *GUIHandler) handleAPIDocsFile(w http.ResponseWriter, r *http.Request) {
	repoRoot, err := docsRepoRoot()
	if err != nil {
		http.Error(w, "Failed to locate repo root", http.StatusInternalServerError)
		return
	}

	relPath := r.URL.Query().Get("path")
	fullPath, err := safeDocPath(repoRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if info.Size() > maxDocBytes {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"path":    relPath,
		"content": string(data),
	})
}

func docsRepoRoot() (string, error) {
	if envRoot := repoRootFromEnv(); envRoot != "" {
		return envRoot, nil
	}

	_, cwd, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return "", err
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	return findRepoRoot(cwd)
}

func repoRootFromEnv() string {
	for _, key := range []string{"GASTOWN_DOCS_ROOT", "GASTOWN_SRC", "GT_ROOT"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		if root, err := findRepoRoot(value); err == nil {
			return root
		}
		if docsOK(value) {
			return value
		}
	}
	return ""
}

func docsOK(root string) bool {
	info, err := os.Stat(filepath.Join(root, "docs"))
	return err == nil && info.IsDir()
}

func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		modPath := filepath.Join(dir, "go.mod")
		if info, err := os.Stat(modPath); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("repo root not found")
}

func safeDocPath(repoRoot, relPath string) (string, error) {
	if relPath == "" {
		return "", errors.New("missing path")
	}
	if strings.Contains(relPath, "\x00") {
		return "", errors.New("invalid path")
	}
	if filepath.IsAbs(relPath) {
		return "", errors.New("absolute paths not allowed")
	}

	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") {
		return "", errors.New("invalid path")
	}

	fullPath := filepath.Join(repoRoot, clean)
	repoRoot = filepath.Clean(repoRoot)
	if fullPath != repoRoot && !strings.HasPrefix(fullPath, repoRoot+string(os.PathSeparator)) {
		return "", errors.New("path escapes repo root")
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext != ".md" && ext != ".tmpl" {
		return "", errors.New("only markdown or tmpl files supported")
	}

	return fullPath, nil
}

func collectMarkdownFiles(repoRoot string) ([]string, error) {
	skipDirs := map[string]bool{
		".git":         true,
		".idea":        true,
		".vscode":      true,
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		"build":        true,
		"out":          true,
		"tmp":          true,
	}

	var files []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".tmpl" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
