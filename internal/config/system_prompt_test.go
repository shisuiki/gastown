package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/templates"
)

func TestResolveSystemPrompt(t *testing.T) {
	// Create a temporary town directory
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	rigPath := filepath.Join(townRoot, "testrig")

	// Create necessary directories
	if err := os.MkdirAll(filepath.Join(townRoot, "settings"), 0755); err != nil {
		t.Fatalf("Failed to create town settings dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, "settings"), 0755); err != nil {
		t.Fatalf("Failed to create rig settings dir: %v", err)
	}

	t.Run("built-in system prompt", func(t *testing.T) {
		// Should return built-in prompt for polecat role
		prompt, err := ResolveSystemPrompt("polecat", townRoot, rigPath)
		if err != nil {
			t.Fatalf("ResolveSystemPrompt failed: %v", err)
		}
		if prompt == "" {
			t.Error("Expected non-empty built-in system prompt for polecat")
		}

		// Verify it's the built-in prompt
		builtinPrompt, err := templates.GetBuiltinSystemPrompt("polecat")
		if err != nil {
			t.Fatalf("Failed to get built-in prompt: %v", err)
		}
		if prompt != builtinPrompt {
			t.Error("Prompt does not match built-in template")
		}
	})

	t.Run("town-level override", func(t *testing.T) {
		// Create town settings with system prompt override
		townSettings := NewTownSettings()
		townSettings.SystemPrompts = map[string]string{
			"polecat": "Custom town-level polecat prompt",
		}
		if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
			t.Fatalf("Failed to save town settings: %v", err)
		}

		prompt, err := ResolveSystemPrompt("polecat", townRoot, rigPath)
		if err != nil {
			t.Fatalf("ResolveSystemPrompt failed: %v", err)
		}
		if prompt != "Custom town-level polecat prompt" {
			t.Errorf("Expected town-level prompt, got: %s", prompt)
		}
	})

	t.Run("rig-level override", func(t *testing.T) {
		// Create rig settings with system prompt override
		rigSettings := NewRigSettings()
		rigSettings.SystemPrompts = map[string]string{
			"polecat": "Custom rig-level polecat prompt",
		}
		if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
			t.Fatalf("Failed to save rig settings: %v", err)
		}

		prompt, err := ResolveSystemPrompt("polecat", townRoot, rigPath)
		if err != nil {
			t.Fatalf("ResolveSystemPrompt failed: %v", err)
		}
		if prompt != "Custom rig-level polecat prompt" {
			t.Errorf("Expected rig-level prompt (highest precedence), got: %s", prompt)
		}
	})

	t.Run("file-based prompt", func(t *testing.T) {
		// Create a custom prompt file
		customPromptFile := filepath.Join(tmpDir, "custom_prompt.md")
		customPromptContent := "This is a custom file-based system prompt"
		if err := os.WriteFile(customPromptFile, []byte(customPromptContent), 0644); err != nil {
			t.Fatalf("Failed to write custom prompt file: %v", err)
		}

		// Update rig settings to use file-based prompt
		rigSettings := NewRigSettings()
		rigSettings.SystemPrompts = map[string]string{
			"polecat": "file:" + customPromptFile,
		}
		if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
			t.Fatalf("Failed to save rig settings: %v", err)
		}

		prompt, err := ResolveSystemPrompt("polecat", townRoot, rigPath)
		if err != nil {
			t.Fatalf("ResolveSystemPrompt failed: %v", err)
		}
		if prompt != customPromptContent {
			t.Errorf("Expected file-based prompt content, got: %s", prompt)
		}
	})

	t.Run("missing role", func(t *testing.T) {
		// Should gracefully return empty string for role without built-in prompt
		prompt, err := ResolveSystemPrompt("nonexistent", townRoot, rigPath)
		if err != nil {
			t.Fatalf("ResolveSystemPrompt should not fail for missing role: %v", err)
		}
		if prompt != "" {
			t.Errorf("Expected empty prompt for missing role, got: %s", prompt)
		}
	})
}

func TestBuildCommandWithPrompt_SystemPrompt(t *testing.T) {
	t.Run("system prompt only", func(t *testing.T) {
		rc := &RuntimeConfig{
			Command:      "claude",
			Args:         []string{"--dangerously-skip-permissions"},
			SystemPrompt: "You are a test agent",
		}

		cmd := rc.BuildCommandWithPrompt("")
		// Should include the system prompt
		if cmd == "claude --dangerously-skip-permissions" {
			t.Error("Expected command to include system prompt")
		}
	})

	t.Run("system prompt with user prompt", func(t *testing.T) {
		rc := &RuntimeConfig{
			Command:      "claude",
			Args:         []string{"--dangerously-skip-permissions"},
			SystemPrompt: "You are a test agent",
		}

		cmd := rc.BuildCommandWithPrompt("User task: do something")
		// Should combine system prompt and user prompt
		if cmd == "claude --dangerously-skip-permissions" {
			t.Error("Expected command to include combined prompts")
		}

		// Verify the command contains a quoted prompt argument
		if !contains(cmd, "You are a test agent") {
			t.Error("Command should include system prompt in final prompt")
		}
	})

	t.Run("no system prompt", func(t *testing.T) {
		rc := &RuntimeConfig{
			Command: "claude",
			Args:    []string{"--dangerously-skip-permissions"},
		}

		cmd := rc.BuildCommandWithPrompt("User task")
		expected := `claude --dangerously-skip-permissions "User task"`
		if cmd != expected {
			t.Errorf("Expected %q, got %q", expected, cmd)
		}
	})
}

func TestAgentSupportsSystemPrompt(t *testing.T) {
	tests := []struct {
		agent    AgentPreset
		supports bool
	}{
		{AgentClaude, true},
		{AgentGemini, true},
		{AgentCodex, true},
		{AgentCursor, true},
		{AgentAuggie, true},
		{AgentAmp, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			info := GetAgentPreset(tt.agent)
			if info == nil {
				t.Fatalf("Agent preset %s not found", tt.agent)
			}
			if info.SupportsSystemPrompt != tt.supports {
				t.Errorf("Expected SupportsSystemPrompt=%v for %s, got %v",
					tt.supports, tt.agent, info.SupportsSystemPrompt)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && anySubstring(s, substr))
}

func anySubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
