package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	if usage.Today != nil {
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

	stats, err := loadClaudeStatsCache()
	if err != nil {
		if usage.Error != "" {
			provider.Error = usage.Error
		} else {
			provider.Error = "No usage data"
		}
		return provider
	}

	date, tokensByModel := stats.latestDailyTokens()
	if len(tokensByModel) == 0 {
		provider.Error = "No daily token data"
		return provider
	}

	totalTokens := 0.0
	for _, val := range tokensByModel {
		totalTokens += val
	}
	label := "Today"
	if date != "" {
		label = fmt.Sprintf("Today (%s)", date)
	}
	provider.Rows = append(provider.Rows, CLIUsageRow{
		Label:  label,
		Tokens: &totalTokens,
	})

	type modelRow struct {
		name   string
		tokens float64
	}
	var models []modelRow
	for model, tokenCount := range tokensByModel {
		models = append(models, modelRow{name: model, tokens: tokenCount})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].tokens > models[j].tokens
	})
	for _, model := range models {
		value := model.tokens
		provider.Rows = append(provider.Rows, CLIUsageRow{
			Label:  model.name,
			Tokens: &value,
		})
	}

	return provider
}

func (h *GUIHandler) fetchCodexUsageProvider() CLIUsageProvider {
	provider := CLIUsageProvider{Provider: "Codex"}

	cmd := strings.TrimSpace(os.Getenv("GT_CODEX_USAGE_CMD"))
	if cmd != "" {
		rows, err := usageRowsFromCommand(cmd)
		if err != nil {
			provider.Error = fmt.Sprintf("GT_CODEX_USAGE_CMD: %v", err)
			return provider
		}
		provider.Rows = rows
		return provider
	}

	tokens, err := fetchCodexDailyTokens()
	if err != nil {
		provider.Error = err.Error()
		return provider
	}
	provider.Rows = []CLIUsageRow{
		{
			Label:  "Today",
			Tokens: &tokens,
		},
	}
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
		weeklyTokens, err := fetchClaudeWeeklyTokensFromStats()
		if err != nil {
			status.Error = err.Error()
			return status
		}
		used := float64(weeklyTokens)
		status.Used = &used
		status.Unit = "tokens"
		return status
	}

	if hasLimitUSD {
		_, weeklyCost, err := fetchClaudeWeeklyTotals()
		if err != nil {
			status.Error = fmt.Sprintf("ccusage: %v", err)
			return status
		}
		used := weeklyCost
		limit := limitUSD
		status.Used = &used
		status.Limit = &limit
		status.Unit = "USD"
		return status
	}

	weeklyTokens, err := fetchClaudeWeeklyTokensFromStats()
	if err != nil {
		status.Error = err.Error()
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
		rateLimit, err := fetchCodexRateLimit()
		if err != nil {
			status.Error = err.Error()
			return status
		}
		status.Percent = &rateLimit.UsedPercent
		if rateLimit.WindowMinutes >= 10080 {
			status.Period = "weekly"
		} else if rateLimit.WindowMinutes > 0 {
			status.Period = fmt.Sprintf("%dm", rateLimit.WindowMinutes)
		}
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

type claudeStatsCache struct {
	DailyModelTokens []struct {
		Date          string             `json:"date"`
		TokensByModel map[string]float64 `json:"tokensByModel"`
	} `json:"dailyModelTokens"`
}

func loadClaudeStatsCache() (*claudeStatsCache, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return nil, fmt.Errorf("HOME not set")
	}
	path := filepath.Join(home, ".claude", "stats-cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var stats claudeStatsCache
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func (c *claudeStatsCache) latestDailyTokens() (string, map[string]float64) {
	if c == nil || len(c.DailyModelTokens) == 0 {
		return "", nil
	}
	latest := c.DailyModelTokens[0]
	latestTime, _ := time.Parse("2006-01-02", latest.Date)
	for _, entry := range c.DailyModelTokens[1:] {
		entryTime, err := time.Parse("2006-01-02", entry.Date)
		if err != nil {
			continue
		}
		if entryTime.After(latestTime) {
			latest = entry
			latestTime = entryTime
		}
	}
	return latest.Date, latest.TokensByModel
}

func fetchClaudeWeeklyTokensFromStats() (int64, error) {
	stats, err := loadClaudeStatsCache()
	if err != nil {
		return 0, err
	}
	if len(stats.DailyModelTokens) == 0 {
		return 0, fmt.Errorf("no daily token data")
	}

	type daily struct {
		date   time.Time
		tokens float64
	}
	var days []daily
	for _, entry := range stats.DailyModelTokens {
		date, err := time.Parse("2006-01-02", entry.Date)
		if err != nil {
			continue
		}
		total := 0.0
		for _, val := range entry.TokensByModel {
			total += val
		}
		days = append(days, daily{date: date, tokens: total})
	}
	if len(days) == 0 {
		return 0, fmt.Errorf("no daily token data")
	}

	sort.Slice(days, func(i, j int) bool {
		return days[i].date.Before(days[j].date)
	})
	start := len(days) - 7
	if start < 0 {
		start = 0
	}
	sum := 0.0
	for i := start; i < len(days); i++ {
		sum += days[i].tokens
	}
	return int64(sum), nil
}

type codexTokenUsage struct {
	InputTokens       float64 `json:"input_tokens"`
	CachedInputTokens float64 `json:"cached_input_tokens"`
	OutputTokens      float64 `json:"output_tokens"`
	ReasoningTokens   float64 `json:"reasoning_output_tokens"`
	TotalTokens       float64 `json:"total_tokens"`
}

type codexRateLimit struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

type codexTokenCountPayload struct {
	Type string `json:"type"`
	Info struct {
		LastTokenUsage codexTokenUsage `json:"last_token_usage"`
	} `json:"info"`
	RateLimits struct {
		Primary   codexRateLimit `json:"primary"`
		Secondary codexRateLimit `json:"secondary"`
	} `json:"rate_limits"`
}

type codexEvent struct {
	Type    string                 `json:"type"`
	Payload codexTokenCountPayload `json:"payload"`
}

func fetchCodexDailyTokens() (float64, error) {
	root := codexSessionsRoot()
	if root == "" {
		return 0, fmt.Errorf("codex sessions not found")
	}
	today := time.Now()
	dir := filepath.Join(root, today.Format("2006"), today.Format("01"), today.Format("02"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("codex sessions not found")
	}

	total := 0.0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var event codexEvent
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				continue
			}
			if event.Type != "event_msg" || event.Payload.Type != "token_count" {
				continue
			}
			usage := event.Payload.Info.LastTokenUsage
			if usage.TotalTokens == 0 {
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens + usage.CachedInputTokens + usage.ReasoningTokens
			}
			total += usage.TotalTokens
		}
		_ = file.Close()
	}

	if total == 0 {
		return 0, fmt.Errorf("no codex token data")
	}
	return total, nil
}

func fetchCodexRateLimit() (codexRateLimit, error) {
	root := codexSessionsRoot()
	if root == "" {
		return codexRateLimit{}, fmt.Errorf("codex sessions not found")
	}
	latestPath, err := findLatestCodexSession(root)
	if err != nil {
		return codexRateLimit{}, err
	}

	file, err := os.Open(latestPath)
	if err != nil {
		return codexRateLimit{}, err
	}
	defer func() { _ = file.Close() }()

	var lastLimit *codexRateLimit
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var event codexEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type != "event_msg" || event.Payload.Type != "token_count" {
			continue
		}
		limit := pickCodexRateLimit(event.Payload.RateLimits.Primary, event.Payload.RateLimits.Secondary)
		lastLimit = &limit
	}
	if lastLimit == nil {
		return codexRateLimit{}, fmt.Errorf("codex rate limits unavailable")
	}
	return *lastLimit, nil
}

func codexSessionsRoot() string {
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	root := filepath.Join(home, ".codex", "sessions")
	if _, err := os.Stat(root); err != nil {
		return ""
	}
	return root
}

func findLatestCodexSession(root string) (string, error) {
	var latestPath string
	var latestMod time.Time
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if latestPath == "" || info.ModTime().After(latestMod) {
			latestPath = path
			latestMod = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if latestPath == "" {
		return "", fmt.Errorf("codex sessions not found")
	}
	return latestPath, nil
}

func pickCodexRateLimit(primary, secondary codexRateLimit) codexRateLimit {
	if secondary.WindowMinutes >= primary.WindowMinutes {
		return secondary
	}
	return primary
}
