package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
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
	Name      string `json:"name"`
	IsCurrent bool   `json:"is_current"`
	LastCommit string `json:"last_commit"`
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

	// Check cache first
	if cached := h.cache.Get("system"); cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

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
	cmd := exec.Command("df", "-B1", "/")
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

	// Cache the result
	h.cache.Set("system", info, SystemCacheTTL)

	json.NewEncoder(w).Encode(info)
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
	cmd := exec.Command("git", "log", "-"+strconv.Itoa(count),
		"--format=%H|%h|%an|%ae|%ar|%s")
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
	branchCmd := exec.Command("git", "branch", "--show-current")
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

	cmd := exec.Command("git", "branch", "-a", "--format=%(refname:short)|%(HEAD)|%(objectname:short)")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"branches": []interface{}{},
			"error":    err.Error(),
		})
		return
	}

	var branches []GitBranch
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) >= 2 {
			branch := GitBranch{
				Name:      parts[0],
				IsCurrent: parts[1] == "*",
			}
			if len(parts) >= 3 {
				branch.LastCommit = parts[2]
			}
			branches = append(branches, branch)
		}
	}

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

	// Get commits with parent info for graph
	cmd := exec.Command("git", "log", "-"+strconv.Itoa(count), "--all",
		"--format=%H|%P|%h|%an|%ar|%s|%D")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"nodes": []interface{}{},
			"error": err.Error(),
		})
		return
	}

	type GraphNode struct {
		Hash      string   `json:"hash"`
		ShortHash string   `json:"short_hash"`
		Parents   []string `json:"parents"`
		Author    string   `json:"author"`
		Date      string   `json:"date"`
		Message   string   `json:"message"`
		Refs      string   `json:"refs"`
	}

	var nodes []GraphNode
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 7)
		if len(parts) >= 6 {
			node := GraphNode{
				Hash:      parts[0],
				ShortHash: parts[2],
				Author:    parts[3],
				Date:      parts[4],
				Message:   parts[5],
			}
			if parts[1] != "" {
				node.Parents = strings.Fields(parts[1])
			}
			if len(parts) >= 7 {
				node.Refs = parts[6]
			}
			nodes = append(nodes, node)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
	})
}

// ClaudeUsage represents Claude Code usage statistics from ccusage.
type ClaudeUsage struct {
	Today      *DailyUsage  `json:"today,omitempty"`
	Totals     *UsageTotals `json:"totals,omitempty"`
	ActiveBlock *BillingBlock `json:"active_block,omitempty"`
	Error      string       `json:"error,omitempty"`
}

// DailyUsage represents a single day's usage.
type DailyUsage struct {
	Date        string         `json:"date"`
	InputTokens int64          `json:"input_tokens"`
	OutputTokens int64         `json:"output_tokens"`
	CacheCreate int64          `json:"cache_create"`
	CacheRead   int64          `json:"cache_read"`
	TotalTokens int64          `json:"total_tokens"`
	TotalCost   float64        `json:"total_cost"`
	Models      []ModelBreakdown `json:"models,omitempty"`
}

// UsageTotals represents cumulative usage totals.
type UsageTotals struct {
	InputTokens int64   `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheCreate int64   `json:"cache_create"`
	CacheRead   int64   `json:"cache_read"`
	TotalTokens int64   `json:"total_tokens"`
	TotalCost   float64 `json:"total_cost"`
}

// ModelBreakdown represents usage for a single model.
type ModelBreakdown struct {
	Model       string  `json:"model"`
	InputTokens int64   `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheCreate int64   `json:"cache_create"`
	CacheRead   int64   `json:"cache_read"`
	Cost        float64 `json:"cost"`
}

// BillingBlock represents an active billing window.
type BillingBlock struct {
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	BurnRate     float64 `json:"burn_rate"`      // cost per hour
	ProjectedCost float64 `json:"projected_cost"` // projected total for this block
}

// handleAPIClaudeUsage returns Claude Code usage statistics via ccusage.
func (h *GUIHandler) handleAPIClaudeUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check cache first
	if cached := h.cache.Get("claude:usage"); cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	usage := ClaudeUsage{}

	// Get daily usage from ccusage
	dailyCmd := exec.Command("npx", "ccusage@latest", "daily", "--json")
	dailyOutput, err := dailyCmd.Output()
	if err == nil {
		var dailyData struct {
			Daily []struct {
				Date        string  `json:"date"`
				InputTokens int64   `json:"inputTokens"`
				OutputTokens int64  `json:"outputTokens"`
				CacheCreationTokens int64 `json:"cacheCreationTokens"`
				CacheReadTokens int64 `json:"cacheReadTokens"`
				TotalTokens int64   `json:"totalTokens"`
				TotalCost   float64 `json:"totalCost"`
				ModelBreakdowns []struct {
					ModelName   string  `json:"modelName"`
					InputTokens int64   `json:"inputTokens"`
					OutputTokens int64  `json:"outputTokens"`
					CacheCreationTokens int64 `json:"cacheCreationTokens"`
					CacheReadTokens int64 `json:"cacheReadTokens"`
					Cost        float64 `json:"cost"`
				} `json:"modelBreakdowns"`
			} `json:"daily"`
			Totals struct {
				InputTokens int64   `json:"inputTokens"`
				OutputTokens int64  `json:"outputTokens"`
				CacheCreationTokens int64 `json:"cacheCreationTokens"`
				CacheReadTokens int64 `json:"cacheReadTokens"`
				TotalTokens int64   `json:"totalTokens"`
				TotalCost   float64 `json:"totalCost"`
			} `json:"totals"`
		}

		if err := json.Unmarshal(dailyOutput, &dailyData); err == nil {
			// Get today's data (last entry)
			if len(dailyData.Daily) > 0 {
				today := dailyData.Daily[len(dailyData.Daily)-1]
				usage.Today = &DailyUsage{
					Date:        today.Date,
					InputTokens: today.InputTokens,
					OutputTokens: today.OutputTokens,
					CacheCreate: today.CacheCreationTokens,
					CacheRead:   today.CacheReadTokens,
					TotalTokens: today.TotalTokens,
					TotalCost:   today.TotalCost,
				}
				for _, m := range today.ModelBreakdowns {
					usage.Today.Models = append(usage.Today.Models, ModelBreakdown{
						Model:       m.ModelName,
						InputTokens: m.InputTokens,
						OutputTokens: m.OutputTokens,
						CacheCreate: m.CacheCreationTokens,
						CacheRead:   m.CacheReadTokens,
						Cost:        m.Cost,
					})
				}
			}

			usage.Totals = &UsageTotals{
				InputTokens: dailyData.Totals.InputTokens,
				OutputTokens: dailyData.Totals.OutputTokens,
				CacheCreate: dailyData.Totals.CacheCreationTokens,
				CacheRead:   dailyData.Totals.CacheReadTokens,
				TotalTokens: dailyData.Totals.TotalTokens,
				TotalCost:   dailyData.Totals.TotalCost,
			}
		}
	}

	// Get active billing block from ccusage blocks
	blocksCmd := exec.Command("npx", "ccusage@latest", "blocks", "--json")
	blocksOutput, err := blocksCmd.Output()
	if err == nil {
		var blocksData struct {
			Blocks []struct {
				StartTime   string `json:"startTime"`
				EndTime     string `json:"endTime"`
				IsActive    bool   `json:"isActive"`
				TotalTokens int64  `json:"totalTokens"`
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

	// Cache for 60 seconds
	h.cache.Set("claude:usage", usage, 60*time.Second)

	json.NewEncoder(w).Encode(usage)
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
