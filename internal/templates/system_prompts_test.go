package templates

import (
	"testing"
)

func TestGetBuiltinSystemPrompt(t *testing.T) {
	tests := []struct {
		role        string
		shouldExist bool
	}{
		{"polecat", true},
		{"mayor", true},
		{"witness", true},
		{"refinery", true},
		{"crew", true},
		{"deacon", true},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			prompt, err := GetBuiltinSystemPrompt(tt.role)
			if err != nil {
				t.Fatalf("GetBuiltinSystemPrompt failed: %v", err)
			}

			if tt.shouldExist {
				if prompt == "" {
					t.Errorf("Expected non-empty prompt for role %s", tt.role)
				}
			} else {
				if prompt != "" {
					t.Errorf("Expected empty prompt for nonexistent role %s, got: %s", tt.role, prompt)
				}
			}
		})
	}
}

func TestHasBuiltinSystemPrompt(t *testing.T) {
	tests := []struct {
		role   string
		exists bool
	}{
		{"polecat", true},
		{"mayor", true},
		{"witness", true},
		{"refinery", true},
		{"crew", true},
		{"deacon", true},
		{"nonexistent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			exists := HasBuiltinSystemPrompt(tt.role)
			if exists != tt.exists {
				t.Errorf("HasBuiltinSystemPrompt(%q) = %v, want %v", tt.role, exists, tt.exists)
			}
		})
	}
}

func TestListBuiltinSystemPrompts(t *testing.T) {
	roles, err := ListBuiltinSystemPrompts()
	if err != nil {
		t.Fatalf("ListBuiltinSystemPrompts failed: %v", err)
	}

	expectedRoles := []string{"polecat", "mayor", "witness", "refinery", "crew", "deacon"}
	if len(roles) != len(expectedRoles) {
		t.Errorf("Expected %d roles, got %d: %v", len(expectedRoles), len(roles), roles)
	}

	// Verify all expected roles are present
	roleMap := make(map[string]bool)
	for _, role := range roles {
		roleMap[role] = true
	}

	for _, expected := range expectedRoles {
		if !roleMap[expected] {
			t.Errorf("Expected role %s not found in list: %v", expected, roles)
		}
	}
}

func TestSystemPromptContent(t *testing.T) {
	// Verify that system prompts contain relevant keywords for each role
	tests := []struct {
		role     string
		keywords []string
	}{
		{"polecat", []string{"autonomous", "gt done", "worker"}},
		{"mayor", []string{"coordinator", "strategic", "Mayor"}},
		{"witness", []string{"monitor", "polecat", "health"}},
		{"refinery", []string{"merge", "queue", "conflict"}},
		{"crew", []string{"crew", "main", "direct"}},
		{"deacon", []string{"Deacon", "town", "infrastructure"}},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			prompt, err := GetBuiltinSystemPrompt(tt.role)
			if err != nil {
				t.Fatalf("GetBuiltinSystemPrompt failed: %v", err)
			}
			if prompt == "" {
				t.Fatalf("Expected non-empty prompt for role %s", tt.role)
			}

			for _, keyword := range tt.keywords {
				if !contains(prompt, keyword) {
					t.Errorf("Expected prompt for %s to contain keyword %q", tt.role, keyword)
				}
			}
		})
	}
}

// Helper to check if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && anySubstring(s, substr)
}

func anySubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
