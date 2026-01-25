package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/workspace"
)

type CICDPageData struct {
	Title      string
	ActivePage string
}

// handleCICD serves the CI/CD status page.
func (h *GUIHandler) handleCICD(w http.ResponseWriter, r *http.Request) {
	data := CICDPageData{
		Title:      "CI/CD",
		ActivePage: "cicd",
	}
	h.renderTemplate(w, "cicd.html", data)
}

type CICDStatus struct {
	UpdatedAt  time.Time             `json:"updated_at"`
	Repo       string                `json:"repo,omitempty"`
	Overall    string                `json:"overall_status"`
	Summary    string                `json:"summary"`
	RecentRuns []CICDRunSummary      `json:"recent_runs"`
	Workflows  []CICDWorkflowSummary `json:"workflows"`
	Reports    CICDReports           `json:"reports"`
	Errors     []string              `json:"errors,omitempty"`
}

type CICDReports struct {
	Internal CICDReport   `json:"internal"`
	External CICDReport   `json:"external"`
	Items    []CICDReport `json:"items,omitempty"`
}

type CICDReport struct {
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Badge     string   `json:"badge"`
	Summary   string   `json:"summary"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	Details   []string `json:"details,omitempty"`
}

type CICDWorkflowSummary struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion,omitempty"`
	StatusGroup string `json:"status_group"`
	Badge       string `json:"badge"`
	LastRunAt   string `json:"last_run_at"`
	LastRunID   int64  `json:"last_run_id,omitempty"`
	Branch      string `json:"branch,omitempty"`
	URL         string `json:"url,omitempty"`
}

type CICDRunSummary struct {
	ID          int64     `json:"id"`
	Workflow    string    `json:"workflow"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion,omitempty"`
	StatusGroup string    `json:"status_group"`
	Badge       string    `json:"badge"`
	Branch      string    `json:"branch,omitempty"`
	Event       string    `json:"event,omitempty"`
	Actor       string    `json:"actor,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	URL         string    `json:"url,omitempty"`
}

type CICDWorkflowFilters struct {
	Workflows []string `json:"workflows"`
	Branches  []string `json:"branches"`
	Statuses  []string `json:"statuses"`
}

type CICDWorkflowResponse struct {
	Workflows []CICDWorkflowSummary `json:"workflows"`
	Runs      []CICDRunSummary      `json:"runs"`
	Filters   CICDWorkflowFilters   `json:"filters"`
	Total     int                   `json:"total"`
	Errors    []string              `json:"errors,omitempty"`
}

type CICDRunDetail struct {
	ID          int64            `json:"id"`
	Workflow    string           `json:"workflow"`
	Title       string           `json:"title"`
	Status      string           `json:"status"`
	Conclusion  string           `json:"conclusion,omitempty"`
	StatusGroup string           `json:"status_group"`
	Badge       string           `json:"badge"`
	Branch      string           `json:"branch,omitempty"`
	Event       string           `json:"event,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	URL         string           `json:"url,omitempty"`
	Jobs        []CICDJobSummary `json:"jobs"`
}

type CICDJobSummary struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// handleAPICICDStatus returns the compact CI/CD status snapshot.
func (h *GUIHandler) handleAPICICDStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	status := h.getCICDStatus()
	json.NewEncoder(w).Encode(status)
}

// handleAPICICDWorkflows returns workflow summaries + run history.
func (h *GUIHandler) handleAPICICDWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	workflow := strings.TrimSpace(r.URL.Query().Get("workflow"))
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := parseIntQuery(r, "limit", 30)
	offset := parseIntQuery(r, "offset", 0)

	runs, errs := h.fetchCICDRunsCached(80)
	workflows := summarizeWorkflows(runs)
	filters := buildCICDFilters(runs)

	filtered := filterCICDRuns(runs, workflow, branch, status)
	total := len(filtered)
	paged := paginateRuns(filtered, offset, limit)

	resp := CICDWorkflowResponse{
		Workflows: workflows,
		Runs:      paged,
		Filters:   filters,
		Total:     total,
		Errors:    errs,
	}

	json.NewEncoder(w).Encode(resp)
}

// handleAPICICDRunDetail returns detailed info for a specific run.
func (h *GUIHandler) handleAPICICDRunDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/cicd/runs/")
	idStr = strings.Trim(idStr, "/")
	if idStr == "" {
		http.Error(w, "Missing run id", http.StatusBadRequest)
		return
	}

	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid run id", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	detail, err := h.fetchCICDRunDetail(runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"run": detail})
}

func (h *GUIHandler) getCICDStatus() CICDStatus {
	cached := h.cache.GetStaleOrRefresh("cicd_status", CICDStatusCacheTTL, func() interface{} {
		status := h.buildCICDStatus()
		return cicdStatusToInterface(status)
	})

	if cached != nil {
		return interfaceToCICDStatus(cached)
	}

	status := h.buildCICDStatus()
	h.cache.Set("cicd_status", cicdStatusToInterface(status), CICDStatusCacheTTL)
	return status
}

func (h *GUIHandler) buildCICDStatus() CICDStatus {
	status := CICDStatus{
		UpdatedAt: time.Now(),
		Overall:   "unknown",
		Summary:   "Unknown",
		Reports: CICDReports{
			Internal: defaultCICDReport("Canary validation"),
			External: defaultCICDReport("Cold-start tests"),
		},
	}

	repoRoot, err := cicdRepoRoot()
	if err != nil {
		status.Errors = append(status.Errors, err.Error())
		return status
	}

	status.Repo = repoSlugFromRoot(repoRoot)

	runs, runErrors := h.fetchCICDRuns(50)
	if len(runErrors) > 0 {
		status.Errors = append(status.Errors, runErrors...)
	}

	if len(runs) > 0 {
		sort.Slice(runs, func(i, j int) bool {
			return runs[i].CreatedAt.After(runs[j].CreatedAt)
		})
		status.RecentRuns = trimRuns(runs, 5)
		status.Workflows = summarizeWorkflows(runs)
		status.Overall = overallStatusFromWorkflows(status.Workflows)
		status.Summary = titleCase(status.Overall)
	}

	externalReport, internalReport := readColdstartReports()
	canaryReport := readCanaryReport()
	status.Reports.Internal = internalReport
	status.Reports.External = externalReport
	status.Reports.Items = []CICDReport{externalReport, internalReport, canaryReport}

	return status
}

func (h *GUIHandler) fetchCICDRunsCached(limit int) ([]CICDRunSummary, []string) {
	cacheKey := fmt.Sprintf("cicd_runs_%d", limit)

	cached := h.cache.GetStaleOrRefresh(cacheKey, CICDWorkflowsCacheTTL, func() interface{} {
		runs, _ := h.fetchCICDRuns(limit)
		return cicdRunsToInterface(runs)
	})

	if cached != nil {
		return interfaceToCICDRuns(cached), nil
	}

	runs, errs := h.fetchCICDRuns(limit)
	h.cache.Set(cacheKey, cicdRunsToInterface(runs), CICDWorkflowsCacheTTL)
	return runs, errs
}

func (h *GUIHandler) fetchCICDRuns(limit int) ([]CICDRunSummary, []string) {
	repoRoot, err := cicdRepoRoot()
	if err != nil {
		return nil, []string{err.Error()}
	}

	args := []string{
		"run", "list",
		"--limit", strconv.Itoa(limit),
		"--json", "databaseId,workflowName,displayTitle,status,conclusion,event,createdAt,updatedAt,url,headBranch",
	}

	cmd, cancel := longCommand("gh", args...)
	defer cancel()
	cmd.Dir = repoRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, []string{"gh CLI not found"}
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, []string{fmt.Sprintf("gh run list failed: %s", detail)}
		}
		return nil, []string{fmt.Sprintf("gh run list failed: %v", err)}
	}

	var payload []ghRun
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return nil, []string{fmt.Sprintf("parse gh run list: %v", err)}
	}

	runs := make([]CICDRunSummary, 0, len(payload))
	for _, run := range payload {
		id := run.DatabaseID
		if id == 0 {
			id = run.ID
		}
		workflow := run.WorkflowName
		if workflow == "" {
			workflow = run.Name
		}
		statusGroup := runStatusGroup(run.Status, run.Conclusion)
		runs = append(runs, CICDRunSummary{
			ID:          id,
			Workflow:    workflow,
			Title:       run.DisplayTitle,
			Status:      run.Status,
			Conclusion:  run.Conclusion,
			StatusGroup: statusGroup,
			Badge:       badgeForStatus(statusGroup),
			Branch:      run.HeadBranch,
			Event:       run.Event,
			CreatedAt:   run.CreatedAt,
			UpdatedAt:   run.UpdatedAt,
			URL:         run.URL,
		})
	}

	return runs, nil
}

func (h *GUIHandler) fetchCICDRunDetail(runID int64) (CICDRunDetail, error) {
	repoRoot, err := cicdRepoRoot()
	if err != nil {
		return CICDRunDetail{}, err
	}

	cmd, cancel := longCommand("gh", "run", "view", strconv.FormatInt(runID, 10),
		"--json", "databaseId,workflowName,displayTitle,status,conclusion,event,createdAt,updatedAt,url,headBranch,jobs")
	defer cancel()
	cmd.Dir = repoRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return CICDRunDetail{}, fmt.Errorf("gh not installed")
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return CICDRunDetail{}, fmt.Errorf("gh run view failed: %s", detail)
		}
		return CICDRunDetail{}, fmt.Errorf("gh run view failed: %w", err)
	}

	var payload ghRunDetail
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return CICDRunDetail{}, fmt.Errorf("parse gh run view: %w", err)
	}

	statusGroup := runStatusGroup(payload.Status, payload.Conclusion)
	detail := CICDRunDetail{
		ID:          payload.DatabaseID,
		Workflow:    payload.WorkflowName,
		Title:       payload.DisplayTitle,
		Status:      payload.Status,
		Conclusion:  payload.Conclusion,
		StatusGroup: statusGroup,
		Badge:       badgeForStatus(statusGroup),
		Branch:      payload.HeadBranch,
		Event:       payload.Event,
		CreatedAt:   payload.CreatedAt,
		UpdatedAt:   payload.UpdatedAt,
		URL:         payload.URL,
	}

	for _, job := range payload.Jobs {
		detail.Jobs = append(detail.Jobs, CICDJobSummary{
			Name:        job.Name,
			Status:      job.Status,
			Conclusion:  job.Conclusion,
			StartedAt:   job.StartedAt,
			CompletedAt: job.CompletedAt,
		})
	}

	return detail, nil
}

type ghRun struct {
	DatabaseID   int64     `json:"databaseId"`
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	WorkflowName string    `json:"workflowName"`
	DisplayTitle string    `json:"displayTitle"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	Event        string    `json:"event"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	HeadBranch   string    `json:"headBranch"`
}

type ghRunDetail struct {
	DatabaseID   int64     `json:"databaseId"`
	WorkflowName string    `json:"workflowName"`
	DisplayTitle string    `json:"displayTitle"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	Event        string    `json:"event"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	HeadBranch   string    `json:"headBranch"`
	Jobs         []struct {
		Name        string    `json:"name"`
		Status      string    `json:"status"`
		Conclusion  string    `json:"conclusion"`
		StartedAt   time.Time `json:"startedAt"`
		CompletedAt time.Time `json:"completedAt"`
	} `json:"jobs"`
}

func summarizeWorkflows(runs []CICDRunSummary) []CICDWorkflowSummary {
	if len(runs) == 0 {
		return nil
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})

	seen := make(map[string]CICDWorkflowSummary)
	order := make([]string, 0)

	for _, run := range runs {
		if _, ok := seen[run.Workflow]; ok {
			continue
		}
		summary := CICDWorkflowSummary{
			Name:        run.Workflow,
			Status:      run.Status,
			Conclusion:  run.Conclusion,
			StatusGroup: run.StatusGroup,
			Badge:       badgeForStatus(run.StatusGroup),
			LastRunAt:   run.CreatedAt.Format(time.RFC3339),
			LastRunID:   run.ID,
			Branch:      run.Branch,
			URL:         run.URL,
		}
		seen[run.Workflow] = summary
		order = append(order, run.Workflow)
	}

	workflows := make([]CICDWorkflowSummary, 0, len(seen))
	for _, name := range order {
		workflows = append(workflows, seen[name])
	}

	return workflows
}

func overallStatusFromWorkflows(workflows []CICDWorkflowSummary) string {
	if len(workflows) == 0 {
		return "unknown"
	}

	degraded := false
	for _, wf := range workflows {
		switch wf.StatusGroup {
		case "failing":
			return "failing"
		case "degraded", "running", "queued", "unknown":
			degraded = true
		}
	}

	if degraded {
		return "degraded"
	}
	return "passing"
}

func runStatusGroup(status, conclusion string) string {
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	normalizedConclusion := strings.ToLower(strings.TrimSpace(conclusion))

	switch normalizedStatus {
	case "in_progress":
		return "running"
	case "queued":
		return "queued"
	}

	switch normalizedConclusion {
	case "success":
		return "passing"
	case "failure", "cancelled", "timed_out", "action_required", "startup_failure":
		return "failing"
	case "neutral", "skipped", "stale":
		return "degraded"
	}

	if normalizedStatus == "completed" && normalizedConclusion == "" {
		return "unknown"
	}

	return "unknown"
}

func badgeForStatus(group string) string {
	switch group {
	case "passing":
		return "badge-green"
	case "failing":
		return "badge-red"
	case "degraded", "running", "queued", "unknown":
		return "badge-yellow"
	default:
		return "badge-blue"
	}
}

func titleCase(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func trimRuns(runs []CICDRunSummary, limit int) []CICDRunSummary {
	if len(runs) <= limit {
		return runs
	}
	return runs[:limit]
}

func filterCICDRuns(runs []CICDRunSummary, workflow, branch, status string) []CICDRunSummary {
	if workflow == "" && branch == "" && status == "" {
		return runs
	}

	filtered := make([]CICDRunSummary, 0, len(runs))
	for _, run := range runs {
		if workflow != "" && !strings.EqualFold(run.Workflow, workflow) {
			continue
		}
		if branch != "" && !strings.EqualFold(run.Branch, branch) {
			continue
		}
		if status != "" && !strings.EqualFold(run.StatusGroup, status) {
			continue
		}
		filtered = append(filtered, run)
	}
	return filtered
}

func paginateRuns(runs []CICDRunSummary, offset, limit int) []CICDRunSummary {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 30
	}
	if offset >= len(runs) {
		return []CICDRunSummary{}
	}
	end := offset + limit
	if end > len(runs) {
		end = len(runs)
	}
	return runs[offset:end]
}

func buildCICDFilters(runs []CICDRunSummary) CICDWorkflowFilters {
	workflowSet := make(map[string]struct{})
	branchSet := make(map[string]struct{})
	statusSet := make(map[string]struct{})

	for _, run := range runs {
		if run.Workflow != "" {
			workflowSet[run.Workflow] = struct{}{}
		}
		if run.Branch != "" {
			branchSet[run.Branch] = struct{}{}
		}
		if run.StatusGroup != "" {
			statusSet[run.StatusGroup] = struct{}{}
		}
	}

	workflows := setToSortedSlice(workflowSet)
	branches := setToSortedSlice(branchSet)
	statuses := setToSortedSlice(statusSet)

	return CICDWorkflowFilters{
		Workflows: workflows,
		Branches:  branches,
		Statuses:  statuses,
	}
}

func setToSortedSlice(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func defaultCICDReport(title string) CICDReport {
	return CICDReport{
		Title:   title,
		Status:  "unknown",
		Badge:   badgeForStatus("unknown"),
		Summary: "No data",
	}
}

type coldstartCombined struct {
	TestID           string            `json:"test_id"`
	Timestamp        string            `json:"timestamp"`
	Mode             string            `json:"mode"`
	Summary          string            `json:"summary"`
	External         coldstartExternal `json:"external"`
	Internal         coldstartInternal `json:"internal"`
	IssuesDiscovered []string          `json:"issues_discovered"`
}

type coldstartExternal struct {
	Overall          string                 `json:"overall"`
	NightingaleBrief string                 `json:"nightingale_brief"`
	PassCount        int                    `json:"pass_count"`
	FailCount        int                    `json:"fail_count"`
	Checks           map[string]interface{} `json:"checks"`
}

type coldstartInternal struct {
	Status     string                        `json:"status"`
	Brief      string                        `json:"brief"`
	MayorBrief string                        `json:"mayor_brief"`
	Components map[string]coldstartComponent `json:"components"`
}

type coldstartComponent struct {
	Status string `json:"status"`
}

func readColdstartReports() (CICDReport, CICDReport) {
	externalReport := defaultCICDReport("Cold-start external probes")
	internalReport := defaultCICDReport("Cold-start internal assessment")

	root, err := cicdLogRoot()
	if err != nil {
		externalReport.Summary = "Missing GT_ROOT for logs"
		internalReport.Summary = "Missing GT_ROOT for logs"
		return externalReport, internalReport
	}

	dir := filepath.Join(root, "logs", "coldstart-tests")
	if _, err := os.Stat(dir); err != nil {
		externalReport.Summary = "No coldstart test results found"
		internalReport.Summary = "No coldstart test results found"
		return externalReport, internalReport
	}

	combinedPath := filepath.Join(dir, "latest.json")
	if combined, modTime, ok := loadColdstartCombined(combinedPath); ok {
		return buildColdstartReports(combined, modTime)
	}

	if combinedPath, _ := latestColdstartCombined(dir); combinedPath != "" {
		if combined, modTime, ok := loadColdstartCombined(combinedPath); ok {
			return buildColdstartReports(combined, modTime)
		}
	}

	externalPath, externalTime := latestColdstartBySuffix(dir, "-external.json")
	internalPath, internalTime := latestColdstartBySuffix(dir, "-internal.json")

	if externalPath != "" {
		if external, ok := loadColdstartExternal(externalPath); ok {
			externalReport = buildColdstartExternalReport(external, externalTime, "")
		}
	}

	if internalPath != "" {
		if internal, ok := loadColdstartInternal(internalPath); ok {
			internalReport = buildColdstartInternalReport(internal, internalTime, nil)
		}
	}

	if externalPath == "" && internalPath == "" {
		externalReport.Summary = "No coldstart test results found"
		internalReport.Summary = "No coldstart test results found"
	}

	return externalReport, internalReport
}

func loadColdstartCombined(path string) (coldstartCombined, time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return coldstartCombined{}, time.Time{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return coldstartCombined{}, time.Time{}, false
	}
	var payload coldstartCombined
	if err := json.Unmarshal(data, &payload); err != nil {
		return coldstartCombined{}, time.Time{}, false
	}
	return payload, info.ModTime(), true
}

func loadColdstartExternal(path string) (coldstartExternal, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return coldstartExternal{}, false
	}
	var payload coldstartExternal
	if err := json.Unmarshal(data, &payload); err != nil {
		return coldstartExternal{}, false
	}
	return payload, true
}

func loadColdstartInternal(path string) (coldstartInternal, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return coldstartInternal{}, false
	}
	var payload coldstartInternal
	if err := json.Unmarshal(data, &payload); err != nil {
		return coldstartInternal{}, false
	}
	return payload, true
}

func latestColdstartCombined(dir string) (string, time.Time) {
	return latestColdstartByFilter(dir, func(name string) bool {
		if !strings.HasPrefix(name, "coldstart-") || !strings.HasSuffix(name, ".json") {
			return false
		}
		return !strings.HasSuffix(name, "-external.json") && !strings.HasSuffix(name, "-internal.json")
	})
}

func latestColdstartBySuffix(dir, suffix string) (string, time.Time) {
	return latestColdstartByFilter(dir, func(name string) bool {
		return strings.HasPrefix(name, "coldstart-") && strings.HasSuffix(name, suffix)
	})
}

func latestColdstartByFilter(dir string, match func(string) bool) (string, time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", time.Time{}
	}
	var latestPath string
	var latestTime time.Time
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.IsDir() {
			continue
		}
		if !match(entry.Name()) {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestPath = filepath.Join(dir, entry.Name())
		}
	}
	return latestPath, latestTime
}

func buildColdstartReports(payload coldstartCombined, modTime time.Time) (CICDReport, CICDReport) {
	externalReport := buildColdstartExternalReport(payload.External, modTime, payload.Summary)
	internalReport := buildColdstartInternalReport(payload.Internal, modTime, payload.IssuesDiscovered)

	if payload.Timestamp != "" {
		externalReport.UpdatedAt = payload.Timestamp
		internalReport.UpdatedAt = payload.Timestamp
	}
	return externalReport, internalReport
}

func buildColdstartExternalReport(payload coldstartExternal, modTime time.Time, fallbackSummary string) CICDReport {
	report := defaultCICDReport("Cold-start external probes")
	report.UpdatedAt = formatTimestamp("", modTime)

	if payload.NightingaleBrief != "" {
		report.Summary = payload.NightingaleBrief
	} else if payload.PassCount > 0 || payload.FailCount > 0 {
		report.Summary = fmt.Sprintf("External probes: %d passed, %d failed", payload.PassCount, payload.FailCount)
	} else if fallbackSummary != "" {
		report.Summary = fallbackSummary
	}

	report.Status = coldstartExternalStatus(payload)
	report.Badge = badgeForStatus(report.Status)
	report.Details = coldstartCheckDetails(payload.Checks)
	return report
}

func buildColdstartInternalReport(payload coldstartInternal, modTime time.Time, issues []string) CICDReport {
	report := defaultCICDReport("Cold-start internal assessment")
	report.UpdatedAt = formatTimestamp("", modTime)

	brief := strings.TrimSpace(payload.Brief)
	if brief == "" {
		brief = strings.TrimSpace(payload.MayorBrief)
	}
	if brief != "" {
		report.Summary = brief
	}

	report.Status = coldstartInternalStatus(payload.Status)
	report.Badge = badgeForStatus(report.Status)
	report.Details = coldstartComponentDetails(payload.Components, issues)
	return report
}

func coldstartExternalStatus(payload coldstartExternal) string {
	overall := strings.ToLower(strings.TrimSpace(payload.Overall))
	if overall == "failing" || overall == "failed" {
		return "failing"
	}
	if payload.FailCount > 0 {
		return "degraded"
	}
	if overall == "passing" || payload.PassCount > 0 {
		return "passing"
	}
	return "unknown"
}

func coldstartInternalStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "fully_operational", "operational", "passing", "pass", "ok":
		return "passing"
	case "degraded", "warning", "partial":
		return "degraded"
	case "no_response", "failed", "fail", "error":
		return "failing"
	default:
		return "unknown"
	}
}

func coldstartCheckDetails(checks map[string]interface{}) []string {
	if len(checks) == 0 {
		return nil
	}
	keys := make([]string, 0, len(checks))
	for key := range checks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		status := coldstartCheckStatus(checks[key])
		if status == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", key, status))
		if len(lines) >= 8 {
			break
		}
	}
	return lines
}

func coldstartCheckStatus(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]interface{}:
		if status, ok := typed["status"].(string); ok {
			return status
		}
	}
	return ""
}

func coldstartComponentDetails(components map[string]coldstartComponent, issues []string) []string {
	lines := make([]string, 0, len(components)+len(issues))
	if len(components) > 0 {
		keys := make([]string, 0, len(components))
		for key := range components {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("%s: %s", key, components[key].Status))
			if len(lines) >= 6 {
				break
			}
		}
	}
	for _, issue := range issues {
		lines = append(lines, "Issue: "+issue)
		if len(lines) >= 8 {
			break
		}
	}
	return lines
}

func formatTimestamp(raw string, fallback time.Time) string {
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	if !fallback.IsZero() {
		return fallback.Format(time.RFC3339)
	}
	return ""
}

func readCanaryReport() CICDReport {
	report := defaultCICDReport("Canary validation")

	root, err := cicdLogRoot()
	if err != nil {
		report.Summary = "Missing GT_ROOT for logs"
		return report
	}

	dir := filepath.Join(root, "logs", "canary-validation")
	entries, err := os.ReadDir(dir)
	if err != nil {
		report.Summary = "No canary validation checkpoints found"
		return report
	}

	latestPath, latestStamp := latestCheckpoint(entries, dir)
	if latestPath == "" {
		report.Summary = "No canary validation checkpoints found"
		return report
	}

	data, err := os.ReadFile(latestPath)
	if err != nil {
		report.Summary = "Failed to read canary checkpoint"
		return report
	}

	checkpoint, err := parseCheckpointJSON(data)
	if err != nil {
		report.Summary = "Failed to parse canary checkpoint"
		return report
	}

	report.UpdatedAt = checkpoint.Timestamp
	if report.UpdatedAt == "" && !latestStamp.IsZero() {
		report.UpdatedAt = latestStamp.Format(time.RFC3339)
	}

	report.Summary = fmt.Sprintf(
		"Dog timeouts: %d (target %d), session deaths: %d",
		checkpoint.Metrics.DogTimeout,
		checkpoint.Baseline.TargetDogTimeout48H,
		checkpoint.Metrics.SessionDeathsAbnormal,
	)

	report.Details = []string{
		fmt.Sprintf("Period: %dh (cutoff %s)", checkpoint.PeriodHours, checkpoint.Cutoff),
		fmt.Sprintf("Dog exit new: %d", checkpoint.Metrics.DogExitNew),
		fmt.Sprintf("Polecat exit new: %d", checkpoint.Metrics.PolecatExitNew),
		fmt.Sprintf("GT done calls: %d", checkpoint.Metrics.GtDoneCalls),
		fmt.Sprintf("Deacon session: %s", checkpoint.Health.DeaconSession),
	}

	report.Status = "passing"
	if checkpoint.Metrics.SessionDeathsAbnormal > 0 || checkpoint.Metrics.DogTimeout > checkpoint.Baseline.TargetDogTimeout48H {
		report.Status = "degraded"
	}
	if checkpoint.Health.DeaconSession != "alive" {
		report.Status = "failing"
	}

	report.Badge = badgeForStatus(report.Status)
	return report
}

type canaryCheckpoint struct {
	Timestamp   string `json:"timestamp"`
	PeriodHours int    `json:"period_hours"`
	Cutoff      string `json:"cutoff"`
	Metrics     struct {
		DogTimeout             int `json:"dog_timeout"`
		DogExitNew             int `json:"dog_exit_new"`
		PolecatExitNew         int `json:"polecat_exit_new"`
		GtDoneCalls            int `json:"gt_done_calls"`
		SessionDeathsTotal     int `json:"session_deaths_total"`
		SessionDeathsSelfClean int `json:"session_deaths_self_clean"`
		SessionDeathsAbnormal  int `json:"session_deaths_abnormal"`
		TerminationReminders   int `json:"termination_reminders_shown"`
		FalsePositives         int `json:"false_positives"`
	} `json:"metrics"`
	Health struct {
		DeaconSession string `json:"deacon_session"`
	} `json:"health"`
	Baseline struct {
		BaselineDogTimeout48H int `json:"baseline_dog_timeout_48h"`
		TargetDogTimeout48H   int `json:"target_dog_timeout_48h"`
		ReductionTargetPct    int `json:"reduction_target_pct"`
	} `json:"baseline_comparison"`
}

func latestCheckpoint(entries []os.DirEntry, dir string) (string, time.Time) {
	var latestPath string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "checkpoint-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestPath = filepath.Join(dir, entry.Name())
		}
	}

	return latestPath, latestTime
}

func parseCheckpointJSON(data []byte) (canaryCheckpoint, error) {
	var checkpoint canaryCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err == nil {
		return checkpoint, nil
	}

	cleaned := sanitizeCheckpointJSON(data)
	if err := json.Unmarshal(cleaned, &checkpoint); err != nil {
		return checkpoint, err
	}
	return checkpoint, nil
}

func sanitizeCheckpointJSON(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "0" || trimmed == "0," {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return []byte(strings.Join(cleaned, "\n"))
}

func readFileSnippet(path string, maxLines int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	buf := make([]string, 0, maxLines)
	data, err := io.ReadAll(io.LimitReader(file, 4096))
	if err != nil {
		return nil
	}

	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		buf = append(buf, line)
		if len(buf) >= maxLines {
			break
		}
	}

	return buf
}

func cicdRepoRoot() (string, error) {
	for _, key := range []string{"GASTOWN_SRC", "GT_ROOT"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		if root, err := findRepoRoot(value); err == nil {
			return root, nil
		}
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

func cicdLogRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("GT_ROOT")); root != "" {
		return root, nil
	}
	townRoot, cwd, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return "", err
	}
	if townRoot != "" {
		return townRoot, nil
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	return cwd, nil
}

func repoSlugFromRoot(repoRoot string) string {
	cmd, cancel := command("git", "config", "--get", "remote.origin.url")
	defer cancel()
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	remote := strings.TrimSpace(string(output))
	if remote == "" {
		return ""
	}

	remote = strings.TrimSuffix(remote, ".git")
	if strings.Contains(remote, "github.com") {
		parts := strings.Split(remote, "github.com")
		if len(parts) > 1 {
			slug := strings.TrimPrefix(parts[1], ":")
			slug = strings.TrimPrefix(slug, "/")
			return slug
		}
	}

	return filepath.Base(remote)
}

func cicdStatusToInterface(status CICDStatus) interface{} {
	data, _ := json.Marshal(status)
	var v interface{}
	json.Unmarshal(data, &v)
	return v
}

func interfaceToCICDStatus(value interface{}) CICDStatus {
	data, _ := json.Marshal(value)
	var status CICDStatus
	json.Unmarshal(data, &status)
	return status
}

func cicdRunsToInterface(runs []CICDRunSummary) interface{} {
	data, _ := json.Marshal(runs)
	var v interface{}
	json.Unmarshal(data, &v)
	return v
}

func interfaceToCICDRuns(value interface{}) []CICDRunSummary {
	data, _ := json.Marshal(value)
	var runs []CICDRunSummary
	json.Unmarshal(data, &runs)
	return runs
}

func parseIntQuery(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
