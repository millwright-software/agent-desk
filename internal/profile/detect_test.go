package profile

import (
	"os"
	"testing"
)

func TestDetectCurrentProfile(t *testing.T) {
	// Save original env vars
	origAgentdeckProfile := os.Getenv("AGENTDESK_PROFILE")
	origClaudeConfigDir := os.Getenv("CLAUDE_CONFIG_DIR")
	defer func() {
		if origAgentdeckProfile != "" {
			os.Setenv("AGENTDESK_PROFILE", origAgentdeckProfile)
		} else {
			os.Unsetenv("AGENTDESK_PROFILE")
		}
		if origClaudeConfigDir != "" {
			os.Setenv("CLAUDE_CONFIG_DIR", origClaudeConfigDir)
		} else {
			os.Unsetenv("CLAUDE_CONFIG_DIR")
		}
	}()

	tests := []struct {
		name             string
		agentdeskProfile string
		claudeConfigDir  string
		expectedContains string // Expected profile (or substring for default case)
	}{
		{
			name:             "explicit AGENTDESK_PROFILE takes priority",
			agentdeskProfile: "work",
			claudeConfigDir:  "/Users/test/.claude-personal",
			expectedContains: "work",
		},
		{
			name:             "CLAUDE_CONFIG_DIR .claude-work suffix",
			agentdeskProfile: "",
			claudeConfigDir:  "/Users/test/.claude-work",
			expectedContains: "work",
		},
		{
			name:             "CLAUDE_CONFIG_DIR .claude-personal suffix",
			agentdeskProfile: "",
			claudeConfigDir:  "/Users/test/.claude-personal",
			expectedContains: "personal",
		},
		{
			name:             "CLAUDE_CONFIG_DIR with hyphen pattern",
			agentdeskProfile: "",
			claudeConfigDir:  "/opt/claude-production",
			expectedContains: "production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env vars
			os.Unsetenv("AGENTDESK_PROFILE")
			os.Unsetenv("CLAUDE_CONFIG_DIR")

			// Set test env vars
			if tt.agentdeskProfile != "" {
				os.Setenv("AGENTDESK_PROFILE", tt.agentdeskProfile)
			}
			if tt.claudeConfigDir != "" {
				os.Setenv("CLAUDE_CONFIG_DIR", tt.claudeConfigDir)
			}

			result := DetectCurrentProfile()
			if result != tt.expectedContains {
				t.Errorf("DetectCurrentProfile() = %q, want %q", result, tt.expectedContains)
			}
		})
	}
}

func TestDetectCurrentProfile_DefaultFallback(t *testing.T) {
	// Save original env vars
	origAgentdeckProfile := os.Getenv("AGENTDESK_PROFILE")
	origClaudeConfigDir := os.Getenv("CLAUDE_CONFIG_DIR")
	defer func() {
		if origAgentdeckProfile != "" {
			os.Setenv("AGENTDESK_PROFILE", origAgentdeckProfile)
		} else {
			os.Unsetenv("AGENTDESK_PROFILE")
		}
		if origClaudeConfigDir != "" {
			os.Setenv("CLAUDE_CONFIG_DIR", origClaudeConfigDir)
		} else {
			os.Unsetenv("CLAUDE_CONFIG_DIR")
		}
	}()

	// Clear all env vars
	os.Unsetenv("AGENTDESK_PROFILE")
	os.Unsetenv("CLAUDE_CONFIG_DIR")

	result := DetectCurrentProfile()
	// Should return either the config default or "default"
	if result == "" {
		t.Error("DetectCurrentProfile() should not return empty string")
	}
}
