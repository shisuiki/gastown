package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RigConfig represents the config.json structure for a rig.
type RigConfig struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	GitURL    string `json:"git_url"`
	LocalRepo string `json:"local_repo"`
}

// RigInfo contains rig name and its git repo path for the Git page.
type RigInfo struct {
	Name    string `json:"name"`
	RepoDir string `json:"repo_dir"`
	HasRepo bool   `json:"has_repo"`
}

// Version is set from the main package
var Version = "0.2.6"

// Build info - set via ldflags at compile time
var (
	BuildNumber = "dev"      // git commit count or build number
	BuildCommit = "unknown"  // git short hash
	BuildTime   = "unknown"  // build timestamp
	StartTime   = time.Now() // service start time
)

// getGTRoot returns the GT_ROOT directory.
func getGTRoot() string {
	dir := os.Getenv("GT_ROOT")
	if dir == "" {
		dir = os.Getenv("HOME") + "/gt"
	}
	return dir
}

// getRigRepoDir returns the git repo directory for a rig by reading its config.json.
// Returns GT_ROOT if rig is empty, or the rig's local_repo if configured.
func getRigRepoDir(rig string) string {
	gtRoot := getGTRoot()
	if rig == "" {
		return gtRoot
	}

	// Read rig's config.json
	configPath := filepath.Join(gtRoot, rig, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Fall back to rig directory if config not found
		return filepath.Join(gtRoot, rig)
	}

	var config RigConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return filepath.Join(gtRoot, rig)
	}

	// Use local_repo if specified, otherwise fall back to rig directory
	if config.LocalRepo != "" {
		return config.LocalRepo
	}
	return filepath.Join(gtRoot, rig)
}

// getRigsWithRepos returns all rigs that have git repositories.
func getRigsWithRepos() []RigInfo {
	gtRoot := getGTRoot()
	var rigs []RigInfo

	entries, err := os.ReadDir(gtRoot)
	if err != nil {
		return rigs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories and special folders
		if strings.HasPrefix(name, ".") || name == "settings" || name == "mayor" {
			continue
		}

		// Check if this is a rig (has config.json with type="rig")
		configPath := filepath.Join(gtRoot, name, "config.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		var config RigConfig
		if err := json.Unmarshal(data, &config); err != nil || config.Type != "rig" {
			continue
		}

		// Determine repo directory
		repoDir := config.LocalRepo
		if repoDir == "" {
			repoDir = filepath.Join(gtRoot, name)
		}

		// Check if repo has .git
		hasRepo := false
		if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
			hasRepo = true
		}

		rigs = append(rigs, RigInfo{
			Name:    name,
			RepoDir: repoDir,
			HasRepo: hasRepo,
		})
	}

	return rigs
}

// SystemInfo represents system resource information.
type SystemInfo struct {
	Hostname    string  `json:"hostname"`
	OS          string  `json:"os"`
	Arch        string  `json:"arch"`
	CPUs        int     `json:"cpus"`
	GoVersion   string  `json:"go_version"`
	Uptime      string  `json:"uptime"`
	LoadAvg     string  `json:"load_avg"`
	MemTotal    string  `json:"mem_total"`
	MemUsed     string  `json:"mem_used"`
	MemPercent  float64 `json:"mem_percent"`
	DiskTotal   string  `json:"disk_total"`
	DiskUsed    string  `json:"disk_used"`
	DiskPercent float64 `json:"disk_percent"`
	// Build info
	Version       string `json:"version"`
	BuildNumber   string `json:"build_number"`
	BuildCommit   string `json:"build_commit"`
	BuildTime     string `json:"build_time"`
	ServiceUptime string `json:"service_uptime"`
}

// GitCommit represents a git commit.
type GitCommit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Author    string `json:"author"`
	Email     string `json:"email"`
	Date      string `json:"date"`
	Message   string `json:"message"`
	Branch    string `json:"branch,omitempty"`
}

// GitBranch represents a git branch.
type GitBranch struct {
	Name        string `json:"name"`
	IsCurrent   bool   `json:"is_current"`
	LastCommit  string `json:"last_commit"`
	Commit      string `json:"commit,omitempty"`
	ShortCommit string `json:"short_commit,omitempty"`
	Upstream    string `json:"upstream,omitempty"`
	Ahead       int    `json:"ahead,omitempty"`
	Behind      int    `json:"behind,omitempty"`
	IsRemote    bool   `json:"is_remote,omitempty"`
}

// GitCommitDetail represents detailed git commit metadata.
type GitCommitDetail struct {
	Hash         string   `json:"hash"`
	ShortHash    string   `json:"short_hash"`
	Author       string   `json:"author"`
	Email        string   `json:"email"`
	Date         string   `json:"date"`
	RelativeDate string   `json:"relative_date"`
	Message      string   `json:"message"`
	Body         string   `json:"body,omitempty"`
	Parents      []string `json:"parents,omitempty"`
	Refs         string   `json:"refs,omitempty"`
}

// GitDiffFile represents per-file stats in a commit diff.
type GitDiffFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status,omitempty"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
}

// GitTreeEntry represents a file or directory in a git tree listing.
type GitTreeEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Mode string `json:"mode"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// GitBlob represents file content for a git blob.
type GitBlob struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Size      int    `json:"size,omitempty"`
}

// GitCompareCommit represents a commit entry in compare view.
type GitCompareCommit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Author    string `json:"author"`
	Date      string `json:"date"`
	Message   string `json:"message"`
}

// GitSearchCommit represents a commit search hit.
type GitSearchCommit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Author    string `json:"author"`
	Date      string `json:"date"`
	Message   string `json:"message"`
}

// GitSearchMatch represents a file search hit.
type GitSearchMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// GitGraphRow represents a line in the git graph output.
type GitGraphRow struct {
	Graph     string   `json:"graph"`
	Hash      string   `json:"hash,omitempty"`
	ShortHash string   `json:"short_hash,omitempty"`
	Author    string   `json:"author,omitempty"`
	Date      string   `json:"date,omitempty"`
	Message   string   `json:"message,omitempty"`
	Refs      string   `json:"refs,omitempty"`
	Parents   []string `json:"parents,omitempty"`
	GraphOnly bool     `json:"graph_only,omitempty"`
}

// GitPageData is the data for the git page.
type GitPageData struct {
	Title      string
	ActivePage string
}

// handleGit serves the git page.
func (h *GUIHandler) handleGit(w http.ResponseWriter, r *http.Request) {
	data := GitPageData{
		Title:      "Git",
		ActivePage: "git",
	}
	h.renderTemplate(w, "git.html", data)
}

// handleAPIVersion returns the version info.
func (h *GUIHandler) handleAPIVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"version": Version,
	})
}

// handleAPISystem returns system resource information.
func (h *GUIHandler) handleAPISystem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Use stale-while-revalidate: return cached data immediately, refresh in background
	cached := h.cache.GetStaleOrRefresh("system", SystemCacheTTL, func() interface{} {
		return h.fetchSystemInfo()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously (first request only)
	info := h.fetchSystemInfo()
	h.cache.Set("system", info, SystemCacheTTL)
	json.NewEncoder(w).Encode(info)
}

// fetchSystemInfo gathers system information.
func (h *GUIHandler) fetchSystemInfo() SystemInfo {
	info := SystemInfo{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		CPUs:      runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}

	// Hostname
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}

	// Uptime (Linux)
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) > 0 {
			if secs, err := strconv.ParseFloat(parts[0], 64); err == nil {
				d := time.Duration(secs) * time.Second
				days := int(d.Hours()) / 24
				hours := int(d.Hours()) % 24
				mins := int(d.Minutes()) % 60
				if days > 0 {
					info.Uptime = strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
				} else {
					info.Uptime = strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
				}
			}
		}
	}

	// Load average (Linux)
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			info.LoadAvg = parts[0] + ", " + parts[1] + ", " + parts[2]
		}
	}

	// Memory info (Linux)
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var memTotal, memAvailable int64
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					memTotal, _ = strconv.ParseInt(parts[1], 10, 64)
				}
			}
			if strings.HasPrefix(line, "MemAvailable:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					memAvailable, _ = strconv.ParseInt(parts[1], 10, 64)
				}
			}
		}
		if memTotal > 0 {
			memUsed := memTotal - memAvailable
			info.MemTotal = formatBytes(memTotal * 1024)
			info.MemUsed = formatBytes(memUsed * 1024)
			info.MemPercent = float64(memUsed) / float64(memTotal) * 100
		}
	}

	// Disk info
	cmd, cancel := command("df", "-B1", "/")
	defer cancel()
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			parts := strings.Fields(lines[1])
			if len(parts) >= 5 {
				total, _ := strconv.ParseInt(parts[1], 10, 64)
				used, _ := strconv.ParseInt(parts[2], 10, 64)
				info.DiskTotal = formatBytes(total)
				info.DiskUsed = formatBytes(used)
				if total > 0 {
					info.DiskPercent = float64(used) / float64(total) * 100
				}
			}
		}
	}

	// Build info
	info.Version = Version
	info.BuildNumber = BuildNumber
	info.BuildCommit = BuildCommit
	info.BuildTime = BuildTime

	// Service uptime (time since this process started)
	uptime := time.Since(StartTime)
	days := int(uptime.Hours()) / 24
	hours := int(uptime.Hours()) % 24
	mins := int(uptime.Minutes()) % 60
	if days > 0 {
		info.ServiceUptime = strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
	} else if hours > 0 {
		info.ServiceUptime = strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
	} else {
		info.ServiceUptime = strconv.Itoa(mins) + "m"
	}

	return info
}

// handleAPIGitRigs returns rigs with their git repository information.
func (h *GUIHandler) handleAPIGitRigs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rigs := getRigsWithRepos()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rigs":    rigs,
		"gt_root": getGTRoot(),
	})
}

// handleAPIGitCommits returns recent git commits.
func (h *GUIHandler) handleAPIGitCommits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get rig parameter and resolve to actual repo directory
	rig := r.URL.Query().Get("rig")
	countStr := r.URL.Query().Get("count")
	cacheKey := "git:commits:" + rig + ":" + countStr

	// Check cache first
	if cached := h.cache.Get(cacheKey); cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	dir := getRigRepoDir(rig)

	// Get commit count
	count := 30
	if countStr != "" {
		if c, err := strconv.Atoi(countStr); err == nil && c > 0 && c <= 100 {
			count = c
		}
	}

	// Get commits
	cmd, cancel := command("git", "log", "-"+strconv.Itoa(count),
		"--format=%H|%h|%an|%ae|%ar|%s")
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"commits": []interface{}{},
			"error":   err.Error(),
		})
		return
	}

	var commits []GitCommit
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		if len(parts) == 6 {
			commits = append(commits, GitCommit{
				Hash:      parts[0],
				ShortHash: parts[1],
				Author:    parts[2],
				Email:     parts[3],
				Date:      parts[4],
				Message:   parts[5],
			})
		}
	}

	// Get current branch
	branchCmd, branchCancel := command("git", "branch", "--show-current")
	defer branchCancel()
	branchCmd.Dir = dir
	if branchOut, err := branchCmd.Output(); err == nil {
		branch := strings.TrimSpace(string(branchOut))
		if len(commits) > 0 {
			commits[0].Branch = branch
		}
	}

	result := map[string]interface{}{
		"commits": commits,
		"dir":     dir,
	}

	// Cache the result
	h.cache.Set(cacheKey, result, GitCacheTTL)

	json.NewEncoder(w).Encode(result)
}

// handleAPIGitBranches returns git branches.
func (h *GUIHandler) handleAPIGitBranches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	dir := getRigRepoDir(rig)

	localBranches, err := gitLocalBranches(dir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"branches": []interface{}{},
			"error":    err.Error(),
		})
		return
	}

	remoteBranches, err := gitRemoteBranches(dir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"branches": []interface{}{},
			"error":    err.Error(),
		})
		return
	}

	branches := append(localBranches, remoteBranches...)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"branches": branches,
	})
}

// handleAPIGitGraph returns git log in graph format for visualization.
func (h *GUIHandler) handleAPIGitGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	dir := getRigRepoDir(rig)

	count := 50
	if c := r.URL.Query().Get("count"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 && n <= 200 {
			count = n
		}
	}

	graphSep := "\x01"
	format := graphSep + "%H\t%h\t%an\t%ar\t%s\t%D\t%P"

	// Get commits with graph prefix + parent info.
	cmd, cancel := longCommand("git", "log", "-"+strconv.Itoa(count), "--all",
		"--graph", "--decorate", "--date=relative", "--format="+format)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rows":  []interface{}{},
			"error": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"rows": parseGitGraphRows(strings.TrimSpace(string(output)), graphSep),
	})
}

// handleAPIGitCommit returns detailed metadata for a single commit.
func (h *GUIHandler) handleAPIGitCommit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	hash := strings.TrimSpace(r.URL.Query().Get("hash"))
	if hash == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "missing hash",
		})
		return
	}

	dir := getRigRepoDir(rig)
	fieldSep := "\x1f"
	format := strings.Join([]string{
		"%H",
		"%h",
		"%an",
		"%ae",
		"%ad",
		"%ar",
		"%s",
		"%b",
		"%P",
		"%D",
	}, fieldSep)

	cmd, cancel := command("git", "show", "--no-patch", "--date=iso-strict", "--format="+format, hash)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	detail, parseErr := parseGitCommitDetail(strings.TrimRight(string(output), "\n"), fieldSep)
	if parseErr != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": parseErr.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"commit": detail,
	})
}

// handleAPIGitCommitDiff returns file stats + patch for a commit.
func (h *GUIHandler) handleAPIGitCommitDiff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	hash := strings.TrimSpace(r.URL.Query().Get("hash"))
	if hash == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "missing hash",
		})
		return
	}

	dir := getRigRepoDir(rig)

	files, err := gitCommitFiles(dir, hash)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	patch, truncated, err := gitCommitPatch(dir, hash)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"files":     files,
		"patch":     patch,
		"truncated": truncated,
	})
}

// handleAPIGitTree returns a tree listing for a ref/path.
func (h *GUIHandler) handleAPIGitTree(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	path := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if ref == "" {
		ref = "HEAD"
	}

	dir := getRigRepoDir(rig)
	entries, err := gitTreeEntries(dir, ref, path)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"ref":     ref,
		"path":    path,
	})
}

// handleAPIGitBlob returns file content for a ref/path.
func (h *GUIHandler) handleAPIGitBlob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	path := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if ref == "" {
		ref = "HEAD"
	}
	if path == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "missing path",
		})
		return
	}

	dir := getRigRepoDir(rig)
	blob, err := gitBlob(dir, ref, path)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"blob": blob,
	})
}

// handleAPIGitCompare returns commit list + diff for a ref range.
func (h *GUIHandler) handleAPIGitCompare(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	base := strings.TrimSpace(r.URL.Query().Get("base"))
	head := strings.TrimSpace(r.URL.Query().Get("head"))
	if base == "" || head == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "missing base or head",
		})
		return
	}

	dir := getRigRepoDir(rig)

	commits, err := gitCompareCommits(dir, base, head)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	files, err := gitRangeFiles(dir, base, head)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	patch, truncated, err := gitRangePatch(dir, base, head)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"commits":   commits,
		"files":     files,
		"patch":     patch,
		"truncated": truncated,
	})
}

// handleAPIGitSearch returns commit or file search results.
func (h *GUIHandler) handleAPIGitSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rig := r.URL.Query().Get("rig")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if query == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "missing query",
		})
		return
	}
	if mode == "" {
		mode = "commits"
	}
	if ref == "" {
		ref = "HEAD"
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	dir := getRigRepoDir(rig)
	switch mode {
	case "files":
		results, err := gitSearchFiles(dir, ref, path, query, limit)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"files": results,
		})
	default:
		results, err := gitSearchCommits(dir, query, limit)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"commits": results,
		})
	}
}

func parseGitGraphRows(output, graphSep string) []GitGraphRow {
	if output == "" {
		return nil
	}

	var rows []GitGraphRow
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		idx := strings.Index(line, graphSep)
		if idx == -1 {
			rows = append(rows, GitGraphRow{
				Graph:     line,
				GraphOnly: true,
			})
			continue
		}

		graph := strings.TrimRight(line[:idx], " ")
		data := line[idx+len(graphSep):]
		parts := strings.SplitN(data, "\t", 7)
		if len(parts) < 5 {
			rows = append(rows, GitGraphRow{
				Graph:     graph,
				GraphOnly: true,
			})
			continue
		}

		row := GitGraphRow{
			Graph:     graph,
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Date:      parts[3],
			Message:   parts[4],
		}
		if len(parts) > 5 {
			row.Refs = parts[5]
		}
		if len(parts) > 6 && parts[6] != "" {
			row.Parents = strings.Fields(parts[6])
		}
		rows = append(rows, row)
	}

	return rows
}

func parseGitCommitDetail(output, fieldSep string) (*GitCommitDetail, error) {
	parts := strings.Split(output, fieldSep)
	if len(parts) < 9 {
		return nil, fmt.Errorf("unexpected commit detail format")
	}

	detail := &GitCommitDetail{
		Hash:         parts[0],
		ShortHash:    parts[1],
		Author:       parts[2],
		Email:        parts[3],
		Date:         parts[4],
		RelativeDate: parts[5],
		Message:      parts[6],
		Body:         strings.TrimSpace(parts[7]),
	}
	if parts[8] != "" {
		detail.Parents = strings.Fields(parts[8])
	}
	if len(parts) > 9 {
		detail.Refs = parts[9]
	}

	return detail, nil
}

func gitCommitFiles(dir, hash string) ([]GitDiffFile, error) {
	nameStatus, err := gitNameStatus(dir, hash)
	if err != nil {
		return nil, err
	}

	numstats, err := gitNumStats(dir, hash)
	if err != nil {
		return nil, err
	}

	for i := range nameStatus {
		if stat, ok := numstats[nameStatus[i].Path]; ok {
			nameStatus[i].Additions = stat.Additions
			nameStatus[i].Deletions = stat.Deletions
			nameStatus[i].Binary = stat.Binary
		}
	}

	return nameStatus, nil
}

func gitNameStatus(dir, hash string) ([]GitDiffFile, error) {
	cmd, cancel := command("git", "show", "--name-status", "--format=", hash)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []GitDiffFile
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		file := GitDiffFile{Status: status}

		switch {
		case strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C"):
			if len(parts) >= 3 {
				file.OldPath = parts[1]
				file.Path = parts[2]
			}
		default:
			file.Path = parts[1]
		}

		if file.Path == "" && len(parts) > 1 {
			file.Path = parts[len(parts)-1]
		}
		files = append(files, file)
	}

	return files, nil
}

func gitNumStats(dir, hash string) (map[string]GitDiffFile, error) {
	cmd, cancel := command("git", "show", "--numstat", "--format=", hash)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]GitDiffFile)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		additions, additionsBinary := parseGitNumStat(parts[0])
		deletions, deletionsBinary := parseGitNumStat(parts[1])
		path := parts[2]
		if len(parts) >= 4 {
			path = parts[3]
		}
		stats[path] = GitDiffFile{
			Path:      path,
			Additions: additions,
			Deletions: deletions,
			Binary:    additionsBinary || deletionsBinary,
		}
	}

	return stats, nil
}

func parseGitNumStat(value string) (int, bool) {
	if value == "-" {
		return 0, true
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return n, false
}

func gitCommitPatch(dir, hash string) (string, bool, error) {
	const maxDiffBytes = 200000

	cmd, cancel := longCommand("git", "show", "--format=", "--patch", hash)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", false, err
	}

	patch := string(output)
	if len(patch) > maxDiffBytes {
		return patch[:maxDiffBytes] + "\n... (diff truncated)\n", true, nil
	}

	return patch, false, nil
}

func gitCompareCommits(dir, base, head string) ([]GitCompareCommit, error) {
	fieldSep := "\x1f"
	format := strings.Join([]string{
		"%H",
		"%h",
		"%an",
		"%ar",
		"%s",
	}, fieldSep)

	cmd, cancel := command("git", "log", "--date=relative", "--format="+format, base+".."+head)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []GitCompareCommit
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, fieldSep)
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, GitCompareCommit{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Date:      parts[3],
			Message:   parts[4],
		})
	}

	return commits, nil
}

func gitRangeFiles(dir, base, head string) ([]GitDiffFile, error) {
	nameStatus, err := gitRangeNameStatus(dir, base, head)
	if err != nil {
		return nil, err
	}

	numstats, err := gitRangeNumStats(dir, base, head)
	if err != nil {
		return nil, err
	}

	for i := range nameStatus {
		if stat, ok := numstats[nameStatus[i].Path]; ok {
			nameStatus[i].Additions = stat.Additions
			nameStatus[i].Deletions = stat.Deletions
			nameStatus[i].Binary = stat.Binary
		}
	}

	return nameStatus, nil
}

func gitRangeNameStatus(dir, base, head string) ([]GitDiffFile, error) {
	cmd, cancel := command("git", "diff", "--name-status", base+".."+head)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []GitDiffFile
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		file := GitDiffFile{Status: status}

		switch {
		case strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C"):
			if len(parts) >= 3 {
				file.OldPath = parts[1]
				file.Path = parts[2]
			}
		default:
			file.Path = parts[1]
		}

		if file.Path == "" && len(parts) > 1 {
			file.Path = parts[len(parts)-1]
		}
		files = append(files, file)
	}

	return files, nil
}

func gitRangeNumStats(dir, base, head string) (map[string]GitDiffFile, error) {
	cmd, cancel := command("git", "diff", "--numstat", base+".."+head)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]GitDiffFile)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		additions, additionsBinary := parseGitNumStat(parts[0])
		deletions, deletionsBinary := parseGitNumStat(parts[1])
		path := parts[2]
		if len(parts) >= 4 {
			path = parts[3]
		}
		stats[path] = GitDiffFile{
			Path:      path,
			Additions: additions,
			Deletions: deletions,
			Binary:    additionsBinary || deletionsBinary,
		}
	}

	return stats, nil
}

func gitRangePatch(dir, base, head string) (string, bool, error) {
	const maxDiffBytes = 200000

	cmd, cancel := longCommand("git", "diff", base+".."+head)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", false, err
	}

	patch := string(output)
	if len(patch) > maxDiffBytes {
		return patch[:maxDiffBytes] + "\n... (diff truncated)\n", true, nil
	}

	return patch, false, nil
}

func gitSearchCommits(dir, query string, limit int) ([]GitSearchCommit, error) {
	fieldSep := "\x1f"
	format := strings.Join([]string{
		"%H",
		"%h",
		"%an",
		"%ar",
		"%s",
	}, fieldSep)

	args := []string{
		"log",
		"--all",
		"--date=relative",
		"--regexp-ignore-case",
		"--grep=" + query,
		"--format=" + format,
		"--max-count=" + strconv.Itoa(limit),
	}

	cmd, cancel := command("git", args...)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []GitSearchCommit
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, fieldSep)
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, GitSearchCommit{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Date:      parts[3],
			Message:   parts[4],
		})
	}

	return commits, nil
}

func gitSearchFiles(dir, ref, path, query string, limit int) ([]GitSearchMatch, error) {
	args := []string{"grep", "-n", "--full-name", "-e", query, ref}
	if path != "" {
		args = append(args, "--", path)
	}
	cmd, cancel := command("git", args...)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		// git grep exits 1 on no matches; return empty result instead of error
		if strings.Contains(err.Error(), "exit status 1") {
			return nil, nil
		}
		return nil, err
	}

	var matches []GitSearchMatch
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		lineNum, _ := strconv.Atoi(parts[1])
		matches = append(matches, GitSearchMatch{
			Path:    parts[0],
			Line:    lineNum,
			Content: parts[2],
		})
		if len(matches) >= limit {
			break
		}
	}

	return matches, nil
}

func gitLocalBranches(dir string) ([]GitBranch, error) {
	cmd, cancel := command("git", "for-each-ref", "refs/heads",
		"--format=%(refname:short)|%(HEAD)|%(objectname)|%(objectname:short)|%(upstream:short)|%(upstream:track)")
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var branches []GitBranch
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 4 {
			continue
		}
		branch := GitBranch{
			Name:        parts[0],
			IsCurrent:   parts[1] == "*",
			Commit:      parts[2],
			ShortCommit: parts[3],
			LastCommit:  parts[3],
		}
		if len(parts) > 4 {
			branch.Upstream = parts[4]
		}
		if len(parts) > 5 {
			branch.Ahead, branch.Behind = parseAheadBehind(parts[5])
		}
		branches = append(branches, branch)
	}

	return branches, nil
}

func gitRemoteBranches(dir string) ([]GitBranch, error) {
	cmd, cancel := command("git", "for-each-ref", "refs/remotes",
		"--format=%(refname:short)|%(objectname)|%(objectname:short)")
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var branches []GitBranch
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		if strings.HasSuffix(name, "/HEAD") {
			continue
		}
		branches = append(branches, GitBranch{
			Name:        name,
			Commit:      parts[1],
			ShortCommit: parts[2],
			LastCommit:  parts[2],
			IsRemote:    true,
		})
	}

	return branches, nil
}

func parseAheadBehind(track string) (ahead, behind int) {
	track = strings.TrimSpace(track)
	track = strings.TrimPrefix(track, "[")
	track = strings.TrimSuffix(track, "]")
	if track == "" {
		return 0, 0
	}
	for _, part := range strings.Split(track, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) != 2 {
			continue
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "ahead":
			ahead = count
		case "behind":
			behind = count
		}
	}
	return ahead, behind
}

func gitTreeEntries(dir, ref, path string) ([]GitTreeEntry, error) {
	args := []string{"ls-tree", "-z", "-l", ref}
	if path != "" {
		args = append(args, path)
	}
	cmd, cancel := command("git", args...)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var entries []GitTreeEntry
	for _, line := range strings.Split(string(output), "\x00") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		meta := strings.Fields(parts[0])
		if len(meta) < 3 {
			continue
		}
		name := parts[1]
		entryPath := name
		if path != "" {
			entryPath = path + "/" + name
		}

		entry := GitTreeEntry{
			Mode: meta[0],
			Type: meta[1],
			Hash: meta[2],
			Name: name,
			Path: entryPath,
		}
		if len(meta) > 3 {
			if size, err := strconv.ParseInt(meta[3], 10, 64); err == nil {
				entry.Size = size
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func gitBlob(dir, ref, path string) (*GitBlob, error) {
	const maxBlobBytes = 200000

	cmd, cancel := longCommand("git", "show", ref+":"+path)
	defer cancel()
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	blob := &GitBlob{
		Path: path,
		Size: len(output),
	}

	if bytes.Contains(output, []byte{0}) {
		blob.Binary = true
		return blob, nil
	}

	if len(output) > maxBlobBytes {
		blob.Content = string(output[:maxBlobBytes]) + "\n... (file truncated)\n"
		blob.Truncated = true
		return blob, nil
	}

	blob.Content = string(output)
	return blob, nil
}

// ClaudeUsage represents Claude Code usage statistics from ccusage.
type ClaudeUsage struct {
	Today       *DailyUsage   `json:"today,omitempty"`
	Totals      *UsageTotals  `json:"totals,omitempty"`
	ActiveBlock *BillingBlock `json:"active_block,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// DailyUsage represents a single day's usage.
type DailyUsage struct {
	Date         string           `json:"date"`
	InputTokens  int64            `json:"input_tokens"`
	OutputTokens int64            `json:"output_tokens"`
	CacheCreate  int64            `json:"cache_create"`
	CacheRead    int64            `json:"cache_read"`
	TotalTokens  int64            `json:"total_tokens"`
	TotalCost    float64          `json:"total_cost"`
	Models       []ModelBreakdown `json:"models,omitempty"`
}

// UsageTotals represents cumulative usage totals.
type UsageTotals struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheCreate  int64   `json:"cache_create"`
	CacheRead    int64   `json:"cache_read"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
}

// ModelBreakdown represents usage for a single model.
type ModelBreakdown struct {
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheCreate  int64   `json:"cache_create"`
	CacheRead    int64   `json:"cache_read"`
	Cost         float64 `json:"cost"`
}

// BillingBlock represents an active billing window.
type BillingBlock struct {
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCost     float64 `json:"total_cost"`
	BurnRate      float64 `json:"burn_rate"`      // cost per hour
	ProjectedCost float64 `json:"projected_cost"` // projected total for this block
}

// handleAPIClaudeUsage returns Claude Code usage statistics via ccusage.
func (h *GUIHandler) handleAPIClaudeUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Use stale-while-revalidate: return cached data immediately, refresh in background
	cached := h.cache.GetStaleOrRefresh("claude_usage", ClaudeUsageCacheTTL, func() interface{} {
		return h.fetchClaudeUsage()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously (first request only)
	usage := h.fetchClaudeUsage()
	h.cache.Set("claude_usage", usage, ClaudeUsageCacheTTL)
	json.NewEncoder(w).Encode(usage)
}

// fetchClaudeUsage gets Claude usage from ccusage CLI.
func (h *GUIHandler) fetchClaudeUsage() ClaudeUsage {
	usage := ClaudeUsage{}

	// Get daily usage from ccusage
	dailyCmd, dailyCancel := longCommand("npx", "ccusage@latest", "daily", "--json")
	defer dailyCancel()
	dailyOutput, err := dailyCmd.Output()
	if err == nil {
		var dailyData struct {
			Daily []struct {
				Date                string  `json:"date"`
				InputTokens         int64   `json:"inputTokens"`
				OutputTokens        int64   `json:"outputTokens"`
				CacheCreationTokens int64   `json:"cacheCreationTokens"`
				CacheReadTokens     int64   `json:"cacheReadTokens"`
				TotalTokens         int64   `json:"totalTokens"`
				TotalCost           float64 `json:"totalCost"`
				ModelBreakdowns     []struct {
					ModelName           string  `json:"modelName"`
					InputTokens         int64   `json:"inputTokens"`
					OutputTokens        int64   `json:"outputTokens"`
					CacheCreationTokens int64   `json:"cacheCreationTokens"`
					CacheReadTokens     int64   `json:"cacheReadTokens"`
					Cost                float64 `json:"cost"`
				} `json:"modelBreakdowns"`
			} `json:"daily"`
			Totals struct {
				InputTokens         int64   `json:"inputTokens"`
				OutputTokens        int64   `json:"outputTokens"`
				CacheCreationTokens int64   `json:"cacheCreationTokens"`
				CacheReadTokens     int64   `json:"cacheReadTokens"`
				TotalTokens         int64   `json:"totalTokens"`
				TotalCost           float64 `json:"totalCost"`
			} `json:"totals"`
		}

		if err := json.Unmarshal(dailyOutput, &dailyData); err == nil {
			// Get today's data (last entry)
			if len(dailyData.Daily) > 0 {
				today := dailyData.Daily[len(dailyData.Daily)-1]
				usage.Today = &DailyUsage{
					Date:         today.Date,
					InputTokens:  today.InputTokens,
					OutputTokens: today.OutputTokens,
					CacheCreate:  today.CacheCreationTokens,
					CacheRead:    today.CacheReadTokens,
					TotalTokens:  today.TotalTokens,
					TotalCost:    today.TotalCost,
				}
				for _, m := range today.ModelBreakdowns {
					usage.Today.Models = append(usage.Today.Models, ModelBreakdown{
						Model:        m.ModelName,
						InputTokens:  m.InputTokens,
						OutputTokens: m.OutputTokens,
						CacheCreate:  m.CacheCreationTokens,
						CacheRead:    m.CacheReadTokens,
						Cost:         m.Cost,
					})
				}
			}

			usage.Totals = &UsageTotals{
				InputTokens:  dailyData.Totals.InputTokens,
				OutputTokens: dailyData.Totals.OutputTokens,
				CacheCreate:  dailyData.Totals.CacheCreationTokens,
				CacheRead:    dailyData.Totals.CacheReadTokens,
				TotalTokens:  dailyData.Totals.TotalTokens,
				TotalCost:    dailyData.Totals.TotalCost,
			}
		}
	}

	// Get active billing block from ccusage blocks
	blocksCmd, blocksCancel := longCommand("npx", "ccusage@latest", "blocks", "--json")
	defer blocksCancel()
	blocksOutput, err := blocksCmd.Output()
	if err == nil {
		var blocksData struct {
			Blocks []struct {
				StartTime   string  `json:"startTime"`
				EndTime     string  `json:"endTime"`
				IsActive    bool    `json:"isActive"`
				TotalTokens int64   `json:"totalTokens"`
				CostUSD     float64 `json:"costUSD"`
				BurnRate    *struct {
					CostPerHour float64 `json:"costPerHour"`
				} `json:"burnRate"`
				Projection *struct {
					TotalCost float64 `json:"totalCost"`
				} `json:"projection"`
			} `json:"blocks"`
		}

		if err := json.Unmarshal(blocksOutput, &blocksData); err == nil {
			// Find active block
			for _, block := range blocksData.Blocks {
				if block.IsActive {
					usage.ActiveBlock = &BillingBlock{
						StartTime:   block.StartTime,
						EndTime:     block.EndTime,
						TotalTokens: block.TotalTokens,
						TotalCost:   block.CostUSD,
					}
					if block.BurnRate != nil {
						usage.ActiveBlock.BurnRate = block.BurnRate.CostPerHour
					}
					if block.Projection != nil {
						usage.ActiveBlock.ProjectedCost = block.Projection.TotalCost
					}
					break
				}
			}
		}
	}

	if usage.Today == nil && usage.Totals == nil {
		usage.Error = "ccusage not available"
	}

	return usage
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64) + " " + string("KMGTPE"[exp]) + "iB"
}
