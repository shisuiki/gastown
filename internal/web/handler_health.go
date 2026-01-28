package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// HealthStatus represents the overall health status.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheckStatus represents the status of an individual check.
type HealthCheckStatus string

const (
	HealthCheckOK      HealthCheckStatus = "ok"
	HealthCheckStale   HealthCheckStatus = "stale"
	HealthCheckMissing HealthCheckStatus = "missing"
	HealthCheckError   HealthCheckStatus = "error"
)

// ColdstartDataCheck contains coldstart data health information.
type ColdstartDataCheck struct {
	Status   HealthCheckStatus `json:"status"`
	LastTest string            `json:"last_test,omitempty"`
	AgeHours float64           `json:"age_hours,omitempty"`
	Message  string            `json:"message,omitempty"`
}

// HealthChecks contains all individual health checks.
type HealthChecks struct {
	Service       HealthCheckStatus  `json:"service"`
	ColdstartData ColdstartDataCheck `json:"coldstart_data"`
	GithubAPI     HealthCheckStatus  `json:"github_api"`
}

// HealthResponse is the response from /api/health.
type HealthResponse struct {
	Status    HealthStatus `json:"status"`
	Checks    HealthChecks `json:"checks"`
	Timestamp string       `json:"timestamp"`
}

// handleAPIHealth returns the health status of the service.
func (h *GUIHandler) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	response := h.buildHealthResponse()

	w.Header().Set("Content-Type", "application/json")

	// Return 503 if unhealthy
	if response.Status == HealthStatusUnhealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

// buildHealthResponse constructs the health check response.
func (h *GUIHandler) buildHealthResponse() HealthResponse {
	checks := HealthChecks{
		Service:       HealthCheckOK, // If we're responding, service is OK
		ColdstartData: checkColdstartData(),
		GithubAPI:     checkGithubAPI(),
	}

	// Determine overall status
	status := HealthStatusHealthy

	// Coldstart data is critical - stale/missing degrades, but doesn't make unhealthy
	if checks.ColdstartData.Status == HealthCheckStale {
		status = HealthStatusDegraded
	} else if checks.ColdstartData.Status == HealthCheckMissing || checks.ColdstartData.Status == HealthCheckError {
		status = HealthStatusDegraded
	}

	// GitHub API is non-critical - only degrades status
	if checks.GithubAPI == HealthCheckError {
		status = HealthStatusDegraded
	}

	return HealthResponse{
		Status:    status,
		Checks:    checks,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// checkColdstartData checks the freshness of coldstart test data.
func checkColdstartData() ColdstartDataCheck {
	result := ColdstartDataCheck{
		Status: HealthCheckMissing,
	}

	root := os.Getenv("GT_ROOT")
	if root == "" {
		result.Message = "GT_ROOT not set"
		return result
	}

	dir := filepath.Join(root, "logs", "coldstart-tests")
	entries, err := os.ReadDir(dir)
	if err != nil {
		result.Message = "No coldstart test directory found"
		return result
	}

	// Find the latest file
	var latestTime time.Time
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.IsDir() {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
		}
	}

	if latestTime.IsZero() {
		result.Message = "No coldstart test results found"
		return result
	}

	// Calculate age
	age := time.Since(latestTime)
	ageHours := age.Hours()

	result.LastTest = latestTime.UTC().Format(time.RFC3339)
	result.AgeHours = ageHours

	// Mark as stale if >24 hours old
	const staleThresholdHours = 24.0
	if ageHours > staleThresholdHours {
		result.Status = HealthCheckStale
		result.Message = "Coldstart data is stale (>24 hours old)"
	} else {
		result.Status = HealthCheckOK
	}

	return result
}

// checkGithubAPI checks if the GitHub API is accessible via gh CLI.
func checkGithubAPI() HealthCheckStatus {
	// Quick check: verify gh CLI is available and authenticated
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return HealthCheckError
	}
	return HealthCheckOK
}
