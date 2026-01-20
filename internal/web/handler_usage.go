package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
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
	if usage.Today == nil {
		if usage.Error != "" {
			provider.Error = usage.Error
		} else {
			provider.Error = "No usage data"
		}
		return provider
	}

	tokens := float64(usage.Today.TotalTokens)
	cost := usage.Today.TotalCost
	label := "Today"
	if usage.Today.Date != "" {
		label = fmt.Sprintf("Today (%s)", usage.Today.Date)
	}
	provider.Rows = append(provider.Rows, CLIUsageRow{
		Label:  label,
		Tokens: &tokens,
		Cost:   &cost,
	})

	if len(usage.Today.Models) > 0 {
		for _, model := range usage.Today.Models {
			modelTokens := model.InputTokens + model.OutputTokens + model.CacheCreate + model.CacheRead
			if modelTokens == 0 && model.Cost == 0 {
				continue
			}
			modelTokenValue := float64(modelTokens)
			modelCost := model.Cost
			provider.Rows = append(provider.Rows, CLIUsageRow{
				Label:  model.Model,
				Tokens: &modelTokenValue,
				Cost:   &modelCost,
			})
		}
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
		switch name {
		case "Claude":
			return h.fetchClaudeWeeklyLimitFromEnv(status)
		case "Codex":
			return h.fetchCodexWeeklyLimitFromEnv(status)
		default:
			status.Error = envVar + " not set"
			return status
		}
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

type claudeDailyReport struct {
	Daily []struct {
		Date                string  `json:"date"`
		InputTokens         int64   `json:"inputTokens"`
		OutputTokens        int64   `json:"outputTokens"`
		CacheCreationTokens int64   `json:"cacheCreationTokens"`
		CacheReadTokens     int64   `json:"cacheReadTokens"`
		TotalTokens         int64   `json:"totalTokens"`
		TotalCost           float64 `json:"totalCost"`
	} `json:"daily"`
}

func fetchClaudeWeeklyTotals() (tokens int64, cost float64, err error) {
	dailyCmd, dailyCancel := longCommand("npx", "ccusage@latest", "daily", "--json")
	defer dailyCancel()
	dailyOutput, err := dailyCmd.Output()
	if err != nil {
		return 0, 0, err
	}

	var report claudeDailyReport
	if err := json.Unmarshal(dailyOutput, &report); err != nil {
		return 0, 0, err
	}
	if len(report.Daily) == 0 {
		return 0, 0, fmt.Errorf("no daily usage data")
	}

	sort.Slice(report.Daily, func(i, j int) bool {
		di := report.Daily[i].Date
		dj := report.Daily[j].Date
		ti, err1 := time.Parse("2006-01-02", di)
		tj, err2 := time.Parse("2006-01-02", dj)
		if err1 != nil || err2 != nil {
			return di < dj
		}
		return ti.Before(tj)
	})

	start := len(report.Daily) - 7
	if start < 0 {
		start = 0
	}
	for i := start; i < len(report.Daily); i++ {
		entry := report.Daily[i]
		entryTokens := entry.TotalTokens
		if entryTokens == 0 {
			entryTokens = entry.InputTokens + entry.OutputTokens + entry.CacheCreationTokens + entry.CacheReadTokens
		}
		tokens += entryTokens
		cost += entry.TotalCost
	}

	return tokens, cost, nil
}

func (h *GUIHandler) fetchClaudeWeeklyLimitFromEnv(status CLILimitStatus) CLILimitStatus {
	limitUSD, hasLimitUSD := parseEnvFloat("GT_CLAUDE_WEEKLY_LIMIT_USD")
	limitTokens, hasLimitTokens := parseEnvFloat("GT_CLAUDE_WEEKLY_LIMIT_TOKENS")

	if !hasLimitUSD && !hasLimitTokens {
		status.Error = "Set GT_CLAUDE_LIMIT_CMD or GT_CLAUDE_WEEKLY_LIMIT_USD/GT_CLAUDE_WEEKLY_LIMIT_TOKENS"
		return status
	}

	weeklyTokens, weeklyCost, err := fetchClaudeWeeklyTotals()
	if err != nil {
		status.Error = fmt.Sprintf("ccusage: %v", err)
		return status
	}

	if hasLimitUSD {
		used := weeklyCost
		limit := limitUSD
		status.Used = &used
		status.Limit = &limit
		status.Unit = "USD"
		return status
	}

	used := float64(weeklyTokens)
	limit := limitTokens
	status.Used = &used
	status.Limit = &limit
	status.Unit = "tokens"
	return status
}

func (h *GUIHandler) fetchCodexWeeklyLimitFromEnv(status CLILimitStatus) CLILimitStatus {
	limitUSD, hasLimitUSD := parseEnvFloat("GT_CODEX_WEEKLY_LIMIT_USD")
	limitTokens, hasLimitTokens := parseEnvFloat("GT_CODEX_WEEKLY_LIMIT_TOKENS")

	if !hasLimitUSD && !hasLimitTokens {
		status.Error = "Set GT_CODEX_LIMIT_CMD or GT_CODEX_WEEKLY_LIMIT_USD/GT_CODEX_WEEKLY_LIMIT_TOKENS"
		return status
	}

	weeklyTokens, weeklyCost, err := fetchCodexWeeklyUsageTotals()
	if err != nil {
		status.Error = err.Error()
		return status
	}

	if hasLimitUSD && weeklyCost != nil {
		used := *weeklyCost
		limit := limitUSD
		status.Used = &used
		status.Limit = &limit
		status.Unit = "USD"
		return status
	}

	if hasLimitTokens && weeklyTokens != nil {
		used := *weeklyTokens
		limit := limitTokens
		status.Used = &used
		status.Limit = &limit
		status.Unit = "tokens"
		return status
	}

	status.Error = "weekly usage data missing"
	return status
}

func fetchCodexWeeklyUsageTotals() (*float64, *float64, error) {
	cmd := strings.TrimSpace(os.Getenv("GT_CODEX_WEEKLY_USAGE_CMD"))
	if cmd == "" {
		return nil, nil, fmt.Errorf("GT_CODEX_WEEKLY_USAGE_CMD not set")
	}

	data, err := runJSONCommand(cmd)
	if err != nil {
		return nil, nil, err
	}

	tokens, cost, err := parseUsageTotals(data)
	if err != nil {
		return nil, nil, err
	}
	return tokens, cost, nil
}

func parseUsageTotals(data []byte) (*float64, *float64, error) {
	var payload struct {
		Tokens       *float64 `json:"tokens"`
		TotalTokens  *float64 `json:"total_tokens"`
		InputTokens  *float64 `json:"input_tokens"`
		OutputTokens *float64 `json:"output_tokens"`
		CacheCreate  *float64 `json:"cache_create"`
		CacheRead    *float64 `json:"cache_read"`
		Cost         *float64 `json:"cost"`
		TotalCost    *float64 `json:"total_cost"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, nil, err
	}

	tokens := payload.Tokens
	if tokens == nil {
		tokens = payload.TotalTokens
	}
	if tokens == nil && (payload.InputTokens != nil || payload.OutputTokens != nil ||
		payload.CacheCreate != nil || payload.CacheRead != nil) {
		sum := 0.0
		if payload.InputTokens != nil {
			sum += *payload.InputTokens
		}
		if payload.OutputTokens != nil {
			sum += *payload.OutputTokens
		}
		if payload.CacheCreate != nil {
			sum += *payload.CacheCreate
		}
		if payload.CacheRead != nil {
			sum += *payload.CacheRead
		}
		tokens = &sum
	}

	cost := payload.Cost
	if cost == nil {
		cost = payload.TotalCost
	}

	if tokens == nil && cost == nil {
		return nil, nil, fmt.Errorf("no usage totals found")
	}

	return tokens, cost, nil
}

func parseEnvFloat(envVar string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(envVar))
	if raw == "" {
		return 0, false
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}
