package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is set from the main package
var Version = "0.2.6"

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

	json.NewEncoder(w).Encode(info)
}

// handleAPIGitCommits returns recent git commits.
func (h *GUIHandler) handleAPIGitCommits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get rig parameter or default to GT_ROOT
	rig := r.URL.Query().Get("rig")
	dir := os.Getenv("GT_ROOT")
	if dir == "" {
		dir = os.Getenv("HOME") + "/gt"
	}
	if rig != "" {
		dir = dir + "/" + rig
	}

	// Get commit count
	countStr := r.URL.Query().Get("count")
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

	json.NewEncoder(w).Encode(map[string]interface{}{
		"commits": commits,
		"dir":     dir,
	})
}

// handleAPIGitBranches returns git branches.
func (h *GUIHandler) handleAPIGitBranches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	dir := os.Getenv("GT_ROOT")
	if dir == "" {
		dir = os.Getenv("HOME") + "/gt"
	}
	if rig := r.URL.Query().Get("rig"); rig != "" {
		dir = dir + "/" + rig
	}

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

	dir := os.Getenv("GT_ROOT")
	if dir == "" {
		dir = os.Getenv("HOME") + "/gt"
	}
	if rig := r.URL.Query().Get("rig"); rig != "" {
		dir = dir + "/" + rig
	}

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
