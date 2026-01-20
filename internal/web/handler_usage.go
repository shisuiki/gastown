package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// CLIUsageSummary represents token usage for CLI providers.
type CLIUsageSummary struct {
	Providers []CLIUsageProvider `json:"providers"`
}

// CLIUsageProvider represents usage details for a single provider.
type CLIUsageProvider struct {
	Provider string        `json:"provider"`
	Rows     []CLIUsageRow `json:"rows,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// CLIUsageRow is a single labeled usage row.
type CLIUsageRow struct {
	Label  string   `json:"label"`
	Tokens *float64 `json:"tokens,omitempty"`
	Cost   *float64 `json:"cost,omitempty"`
}

// CLILimitsResponse represents limit data for CLI providers.
type CLILimitsResponse struct {
	Providers []CLILimitStatus `json:"providers"`
}

// CLILimitStatus represents weekly limit usage for a provider.
type CLILimitStatus struct {
	Provider string   `json:"provider"`
	Period   string   `json:"period,omitempty"`
	Percent  *float64 `json:"percent,omitempty"`
	Used     *float64 `json:"used,omitempty"`
	Limit    *float64 `json:"limit,omitempty"`
	Unit     string   `json:"unit,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// handleAPICLIUsage returns usage summaries for Claude and Codex CLI providers.
func (h *GUIHandler) handleAPICLIUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cached := h.cache.GetStaleOrRefresh("cli_usage", CLIUsageCacheTTL, func() interface{} {
		return h.fetchCLIUsageSummary()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	summary := h.fetchCLIUsageSummary()
	h.cache.Set("cli_usage", summary, CLIUsageCacheTTL)
	json.NewEncoder(w).Encode(summary)
}

// handleAPICLILimits returns weekly limit usage for Claude and Codex.
func (h *GUIHandler) handleAPICLILimits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cached := h.cache.GetStaleOrRefresh("cli_limits", CLILimitsCacheTTL, func() interface{} {
		return h.fetchCLILimits()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	limits := h.fetchCLILimits()
	h.cache.Set("cli_limits", limits, CLILimitsCacheTTL)
	json.NewEncoder(w).Encode(limits)
}

func (h *GUIHandler) fetchCLIUsageSummary() CLIUsageSummary {
	return CLIUsageSummary{
		Providers: []CLIUsageProvider{
			h.fetchClaudeUsageProvider(),
			h.fetchCodexUsageProvider(),
		},
	}
}

func (h *GUIHandler) fetchClaudeUsageProvider() CLIUsageProvider {
	provider := CLIUsageProvider{Provider: "Claude"}

	if cmd := strings.TrimSpace(os.Getenv("GT_CLAUDE_USAGE_CMD")); cmd != "" {
		rows, err := usageRowsFromCommand(cmd)
		if err != nil {
			provider.Error = fmt.Sprintf("GT_CLAUDE_USAGE_CMD: %v", err)
			return provider
		}
		provider.Rows = rows
		return provider
	}

	usage := h.fetchClaudeUsage()
	if usage.Today == nil && usage.ActiveBlock == nil {
		if usage.Error != "" {
			provider.Error = usage.Error
		} else {
			provider.Error = "No usage data"
		}
		return provider
	}

	if usage.Today != nil {
		tokens := float64(usage.Today.TotalTokens)
		cost := usage.Today.TotalCost
		provider.Rows = append(provider.Rows, CLIUsageRow{
			Label:  "Today",
			Tokens: &tokens,
			Cost:   &cost,
		})
	}

	if usage.ActiveBlock != nil {
		tokens := float64(usage.ActiveBlock.TotalTokens)
		cost := usage.ActiveBlock.TotalCost
		provider.Rows = append(provider.Rows, CLIUsageRow{
			Label:  "Block",
			Tokens: &tokens,
			Cost:   &cost,
		})
	}

	return provider
}

func (h *GUIHandler) fetchCodexUsageProvider() CLIUsageProvider {
	provider := CLIUsageProvider{Provider: "Codex"}

	cmd := strings.TrimSpace(os.Getenv("GT_CODEX_USAGE_CMD"))
	if cmd == "" {
		provider.Error = "GT_CODEX_USAGE_CMD not set"
		return provider
	}

	rows, err := usageRowsFromCommand(cmd)
	if err != nil {
		provider.Error = fmt.Sprintf("GT_CODEX_USAGE_CMD: %v", err)
		return provider
	}

	provider.Rows = rows
	return provider
}

func (h *GUIHandler) fetchCLILimits() CLILimitsResponse {
	return CLILimitsResponse{
		Providers: []CLILimitStatus{
			h.fetchLimitProvider("Claude", "GT_CLAUDE_LIMIT_CMD"),
			h.fetchLimitProvider("Codex", "GT_CODEX_LIMIT_CMD"),
		},
	}
}

func (h *GUIHandler) fetchLimitProvider(name, envVar string) CLILimitStatus {
	status := CLILimitStatus{Provider: name, Period: "weekly"}

	cmd := strings.TrimSpace(os.Getenv(envVar))
	if cmd == "" {
		status.Error = envVar + " not set"
		return status
	}

	data, err := runJSONCommand(cmd)
	if err != nil {
		status.Error = fmt.Sprintf("%s: %v", envVar, err)
		return status
	}

	parsed, err := parseLimitStatus(data)
	if err != nil {
		status.Error = fmt.Sprintf("%s: %v", envVar, err)
		return status
	}

	parsed.Provider = name
	if parsed.Period == "" {
		parsed.Period = "weekly"
	}
	return parsed
}

func usageRowsFromCommand(cmdLine string) ([]CLIUsageRow, error) {
	data, err := runJSONCommand(cmdLine)
	if err != nil {
		return nil, err
	}
	return parseUsageRows(data)
}

func parseUsageRows(data []byte) ([]CLIUsageRow, error) {
	var rows []CLIUsageRow
	if err := json.Unmarshal(data, &rows); err == nil && len(rows) > 0 {
		return rows, nil
	}

	var payload struct {
		Rows   []CLIUsageRow `json:"rows"`
		Label  string        `json:"label"`
		Tokens *float64      `json:"tokens"`
		Cost   *float64      `json:"cost"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if len(payload.Rows) > 0 {
		return payload.Rows, nil
	}
	if payload.Tokens != nil || payload.Cost != nil || payload.Label != "" {
		label := payload.Label
		if label == "" {
			label = "Usage"
		}
		return []CLIUsageRow{{
			Label:  label,
			Tokens: payload.Tokens,
			Cost:   payload.Cost,
		}}, nil
	}

	return nil, fmt.Errorf("no usage rows found")
}

func parseLimitStatus(data []byte) (CLILimitStatus, error) {
	var status CLILimitStatus
	if err := json.Unmarshal(data, &status); err != nil {
		var statuses []CLILimitStatus
		if err2 := json.Unmarshal(data, &statuses); err2 != nil || len(statuses) == 0 {
			return status, err
		}
		status = statuses[0]
	}

	if status.Percent == nil || (status.Used == nil && status.Limit == nil) {
		var payload struct {
			Percent      *float64 `json:"percent"`
			WeeklyPct    *float64 `json:"weekly_percent"`
			PercentUsed  *float64 `json:"percent_used"`
			Used         *float64 `json:"used"`
			Limit        *float64 `json:"limit"`
			Unit         string   `json:"unit"`
			Period       string   `json:"period"`
			Description  string   `json:"description"`
			UsagePercent *float64 `json:"usage_percent"`
		}
		if err := json.Unmarshal(data, &payload); err == nil {
			if status.Percent == nil {
				switch {
				case payload.Percent != nil:
					status.Percent = payload.Percent
				case payload.WeeklyPct != nil:
					status.Percent = payload.WeeklyPct
				case payload.PercentUsed != nil:
					status.Percent = payload.PercentUsed
				case payload.UsagePercent != nil:
					status.Percent = payload.UsagePercent
				}
			}
			if status.Used == nil {
				status.Used = payload.Used
			}
			if status.Limit == nil {
				status.Limit = payload.Limit
			}
			if status.Unit == "" {
				status.Unit = payload.Unit
			}
			if status.Period == "" {
				status.Period = payload.Period
			}
			if status.Error == "" && payload.Description != "" {
				status.Error = payload.Description
			}
		}
	}

	if status.Percent == nil && status.Used != nil && status.Limit != nil && *status.Limit > 0 {
		pct := (*status.Used / *status.Limit) * 100
		status.Percent = &pct
	}

	return status, nil
}

func runJSONCommand(cmdLine string) ([]byte, error) {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	cmd, cancel := longCommand(parts[0], parts[1:]...)
	defer cancel()
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("command failed: %s", msg)
	}
	if len(output) == 0 {
		return nil, fmt.Errorf("command returned empty output")
	}
	return output, nil
}
