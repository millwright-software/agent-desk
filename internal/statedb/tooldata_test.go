package statedb

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToolDataRoundtripFlag(t *testing.T) {
	detectedAt := time.Unix(1750000000, 0)
	blob := MarshalToolData(
		"claude-sid", detectedAt, "claude-fable-5",
		"", time.Time{},
		nil, "",
		"", time.Time{},
		"", time.Time{},
		[]string{"mcp-a"},
		nil, "ocean", 3, 1750009999, // FlagParkedBlue, lastLogActivity
	)

	claudeSID, claudeAt, claudeModel,
		_, _,
		_, _,
		_, _,
		_, _,
		mcps,
		_, colorScheme, flag, lastLog := UnmarshalToolData(blob)

	if claudeSID != "claude-sid" {
		t.Errorf("claudeSID = %q, want claude-sid", claudeSID)
	}
	if !claudeAt.Equal(detectedAt) {
		t.Errorf("claudeAt = %v, want %v", claudeAt, detectedAt)
	}
	if claudeModel != "claude-fable-5" {
		t.Errorf("claudeModel = %q, want claude-fable-5", claudeModel)
	}
	if len(mcps) != 1 || mcps[0] != "mcp-a" {
		t.Errorf("mcps = %v, want [mcp-a]", mcps)
	}
	if colorScheme != "ocean" {
		t.Errorf("colorScheme = %q, want ocean", colorScheme)
	}
	if flag != 3 {
		t.Errorf("flag = %d, want 3", flag)
	}
	if lastLog != 1750009999 {
		t.Errorf("lastLogActivity = %d, want 1750009999", lastLog)
	}

	// Clean session stays clean
	blob = MarshalToolData(
		"", time.Time{}, "", "", time.Time{}, nil, "",
		"", time.Time{}, "", time.Time{},
		nil, nil, "", 0, 0,
	)
	_, _, _, _, _, _, _, _, _, _, _, _, _, _, flag, lastLog = UnmarshalToolData(blob)
	if flag != 0 || lastLog != 0 {
		t.Errorf("expected clean flag/lastLog, got %d/%d", flag, lastLog)
	}
}

// TestToolDataLegacyParkedMigration verifies an old row that stored the
// parked bool (before the Flag enum existed) maps to FlagParkedRed (2).
func TestToolDataLegacyParkedMigration(t *testing.T) {
	legacy, _ := json.Marshal(map[string]any{"parked": true})
	_, _, _, _, _, _, _, _, _, _, _, _, _, _, flag, _ := UnmarshalToolData(legacy)
	if flag != 2 {
		t.Errorf("legacy parked should migrate to flag 2 (FlagParkedRed), got %d", flag)
	}
}
