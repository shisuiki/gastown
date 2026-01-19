package models

import (
	"strings"
	"regexp"

	"github.com/steveyegge/gastown/internal/util"
)

var knownClaudeModels = []string{"claude-haiku", "claude-sonnet", "claude-opus"}

// Discover returns the list of available model IDs for the given agent.
// Returns an empty slice if no models are discovered or agent is unknown.
// Errors are logged but not returned; the function returns empty slice on failure.
func Discover(agent string) []string {
	switch agent {
	case "claude":
		return discoverClaude()
	case "gemini":
		return discoverGemini()
	case "codex":
		return discoverCodex()
	case "cursor":
		return discoverCursor()
	case "auggie":
		return discoverAuggie()
	case "amp":
		return discoverAmp()
	default:
		return nil
	}
}

// discoverClaude discovers available Claude models by running `claude --help`.
func discoverClaude() []string {
	// Try `claude --models` first (if supported)
	out, err := util.ExecWithOutput("", "claude", "--models")
	if err == nil {
		models := parseClaudeModels(out)
		if len(models) > 0 {
			return models
		}
	}
	// Fallback to `claude --help` and parse
	out, err = util.ExecWithOutput("", "claude", "--help")
	if err != nil {
		// If CLI unavailable, return known models as fallback
		return knownClaudeModels
	}
	models := parseClaudeHelp(out)
	if len(models) == 0 {
		return knownClaudeModels
	}
	return models
}

// parseClaudeModels parses output of `claude --models`.
// Expected output lines containing model identifiers.
func parseClaudeModels(output string) []string {
	var models []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Usage:") {
			continue
		}
		// TODO: implement proper parsing
		// For now, extract potential model IDs
		if strings.Contains(line, "haiku") || strings.Contains(line, "Haiku") {
			models = append(models, "claude-haiku")
		}
		if strings.Contains(line, "sonnet") || strings.Contains(line, "Sonnet") {
			models = append(models, "claude-sonnet")
		}
		if strings.Contains(line, "opus") || strings.Contains(line, "Opus") {
			models = append(models, "claude-opus")
		}
	}
	return deduplicate(models)
}

// parseClaudeHelp parses `claude --help` output for model references.
func parseClaudeHelp(output string) []string {
	// Look for --model flag description line
	var models []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "--model") && strings.Contains(line, "MODEL") {
			// Extract examples from line like "e.g. 'sonnet' or 'opus'"
			// Use regex to capture words in single quotes
			re := regexp.MustCompile(`'([^']+)'`)
			matches := re.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) > 1 {
					model := match[1]
					// Normalize to claude- prefix
					if !strings.HasPrefix(model, "claude-") {
						model = "claude-" + model
					}
					models = append(models, model)
				}
			}
			// Also look for parentheses with comma-separated list
			start := strings.Index(line, "(")
			end := strings.Index(line, ")")
			if start > 0 && end > start {
				modelsList := line[start+1 : end]
				parts := strings.Split(modelsList, ",")
				for _, part := range parts {
					model := strings.TrimSpace(part)
					if model != "" && !strings.Contains(model, " ") {
						if !strings.HasPrefix(model, "claude-") {
							model = "claude-" + model
						}
						models = append(models, model)
					}
				}
			}
		}
	}
	models = deduplicate(models)
	if len(models) == 0 {
		// Fallback to known models
		return knownClaudeModels
	}
	return models
}

// discoverGemini discovers available Gemini models.
func discoverGemini() []string {
	// TODO: implement
	return nil
}

// discoverCodex discovers available Codex models.
func discoverCodex() []string {
	// TODO: implement
	return nil
}

// discoverCursor discovers available Cursor models.
func discoverCursor() []string {
	// TODO: implement
	return nil
}

// discoverAuggie discovers available Auggie models.
func discoverAuggie() []string {
	// TODO: implement
	return nil
}

// discoverAmp discovers available AMP models.
func discoverAmp() []string {
	// TODO: implement
	return nil
}

// deduplicate removes duplicate strings from slice.
func deduplicate(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, v := range slice {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}