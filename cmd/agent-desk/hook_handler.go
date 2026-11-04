package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/millwright-software/agent-desk/internal/session"
	"github.com/millwright-software/agent-desk/internal/statedb"
)

// hookPayload represents the JSON payload Claude Code sends to hooks via stdin.
// Only the fields we need are decoded; unknown fields are ignored.
type hookPayload struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	Source        string          `json:"source"`
	Matcher       json.RawMessage `json:"matcher,omitempty"`
}

// hookStatusFile is the JSON written to ~/.agent-desk/hooks/{instance_id}.json
type hookStatusFile struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	Event     string `json:"event"`
	Timestamp int64  `json:"ts"`
}

// mapEventToStatus maps a Claude Code hook event to an agent-desk status string.
// Status semantics in agent-desk:
//   - "running" = Claude is actively processing (green)
//   - "waiting" = Claude is at the prompt, waiting for user input (orange)
//   - "dead"    = Session ended
func mapEventToStatus(event string) string {
	switch event {
	case "SessionStart":
		return "waiting" // Claude at initial prompt, waiting for user input
	case "UserPromptSubmit":
		return "running" // User sent prompt, Claude is processing
	case "Stop":
		return "waiting" // Claude finished, back at prompt waiting for user
	case "PermissionRequest":
		return "waiting" // Claude needs permission approval, or asked a question (AskUserQuestion)
	case "PostToolUse":
		// A tool just ran, so Claude is going again. This is what ends a
		// PermissionRequest: you approved, or answered the question, and the
		// tool completed. Without it nothing fires until Stop, and the session
		// would read as waiting for the whole rest of the turn.
		return "running"
	case "Notification":
		// Notification events with permission_prompt|elicitation_dialog matcher
		// are mapped to "waiting" by the caller after checking the matcher.
		// Default notification is informational, treat as no status change.
		return ""
	case "SessionEnd":
		return "dead"
	default:
		return ""
	}
}

// handleHookHandler processes a Claude Code hook event.
// Reads JSON from stdin, maps the event to a status, and writes a status file.
// Always exits 0 to avoid blocking Claude Code.
func handleHookHandler() {
	instanceID := os.Getenv("AGENTDESK_INSTANCE_ID")
	if instanceID == "" {
		// No instance ID means this Claude session isn't managed by agent-desk.
		// Exit silently without error.
		return
	}

	// A Claude Code background session (the daemon's bg-spare workers) can
	// carry a pane's AGENTDESK_INSTANCE_ID (likely inherited from the pane
	// that started it). Its events are not that pane's: on 11/3 one such
	// session's conversation ID landed on L1-OC Counter, and the hook path swapped L1's stored
	// conversation ID for the background one, so Shift+R would have resumed
	// the wrong conversation. Drop events from anything under a bg worker.
	if runningUnderClaudeBackground() {
		return
	}

	// Read stdin (Claude sends hook payload as JSON)
	data, err := io.ReadAll(os.Stdin)
	if err != nil || len(data) == 0 {
		return
	}

	var payload hookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}

	// Map event to status
	status := mapEventToStatus(payload.HookEventName)

	// Special handling for Notification events: only map to "waiting" if
	// the matcher indicates a permission prompt or elicitation dialog
	if payload.HookEventName == "Notification" && payload.Matcher != nil {
		var matcher string
		if err := json.Unmarshal(payload.Matcher, &matcher); err == nil {
			if matcher == "permission_prompt" || matcher == "elicitation_dialog" {
				status = "waiting"
			}
		}
	}

	if status == "" {
		// Unknown or unhandled event, nothing to write
		return
	}

	now := time.Now()

	// The inbox table is the real destination (see statedb.HookInbox). The
	// per-instance file below is only the fallback if the table cannot be
	// opened or written, so a TUI from before the inbox still sees something.
	if inbox, err := statedb.OpenHookInbox(session.HookInboxPath()); err == nil {
		werr := inbox.Record(statedb.HookEvent{
			InstanceID: instanceID, Status: status, Event: payload.HookEventName,
			SessionID: payload.SessionID, At: now,
		})
		_ = inbox.Close()
		if werr == nil {
			return
		}
	}

	// Ensure hooks directory exists
	hooksDir := getHooksDir()
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return
	}

	// Build status file
	statusFile := hookStatusFile{
		Status:    status,
		SessionID: payload.SessionID,
		Event:     payload.HookEventName,
		Timestamp: now.Unix(),
	}

	jsonData, err := json.Marshal(statusFile)
	if err != nil {
		return
	}

	// Atomic write: temp file + rename
	filePath := filepath.Join(hooksDir, instanceID+".json")
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return
	}
	_ = os.Rename(tmpPath, filePath)
}

// getHooksDir returns the path to the hooks status directory.
func getHooksDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".agent-desk", "hooks")
	}
	return filepath.Join(home, ".agent-desk", "hooks")
}

// cleanStaleHookFiles removes hook status files older than 24 hours.
func cleanStaleHookFiles() {
	if inbox, err := statedb.OpenHookInbox(session.HookInboxPath()); err == nil {
		if n, err := inbox.Count(); err == nil {
			fmt.Printf("Hook events: %d instances (in %s)\n", n, session.HookInboxPath())
		}
		_ = inbox.Close()
	}
	hooksDir := getHooksDir()
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(hooksDir, entry.Name()))
		}
	}
}

// handleHooks handles the "hooks" CLI subcommand for manual hook management.
func handleHooks(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: agent-desk hooks <install|uninstall|status>")
		os.Exit(1)
	}

	switch args[0] {
	case "install":
		handleHooksInstall()
	case "uninstall":
		handleHooksUninstall()
	case "status":
		handleHooksStatus()
	default:
		fmt.Fprintf(os.Stderr, "Unknown hooks subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: agent-desk hooks <install|uninstall|status>")
		os.Exit(1)
	}
}

func handleHooksInstall() {
	configDir := getClaudeConfigDirForHooks()
	installed, err := session.InjectClaudeHooks(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error installing hooks: %v\n", err)
		os.Exit(1)
	}
	if installed {
		fmt.Println("Claude Code hooks installed successfully.")
		fmt.Printf("Config: %s/settings.json\n", configDir)
	} else {
		fmt.Println("Claude Code hooks are already installed.")
	}
}

func handleHooksUninstall() {
	configDir := getClaudeConfigDirForHooks()
	removed, err := session.RemoveClaudeHooks(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error removing hooks: %v\n", err)
		os.Exit(1)
	}
	if removed {
		fmt.Println("Claude Code hooks removed successfully.")
	} else {
		fmt.Println("No agent-desk hooks found to remove.")
	}
}

func handleHooksStatus() {
	// Clean up stale hook files while checking status
	cleanStaleHookFiles()

	configDir := getClaudeConfigDirForHooks()
	installed := session.CheckClaudeHooksInstalled(configDir)

	if installed {
		fmt.Println("Status: INSTALLED")
		fmt.Printf("Config: %s/settings.json\n", configDir)
	} else {
		fmt.Println("Status: NOT INSTALLED")
		fmt.Println("Run 'agent-desk hooks install' to install.")
	}

	// Show hook status files
	if inbox, err := statedb.OpenHookInbox(session.HookInboxPath()); err == nil {
		if n, err := inbox.Count(); err == nil {
			fmt.Printf("Hook events: %d instances (in %s)\n", n, session.HookInboxPath())
		}
		_ = inbox.Close()
	}
	hooksDir := getHooksDir()
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		return
	}

	activeCount := 0
	cutoff := time.Now().Add(-5 * time.Second)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			activeCount++
		}
	}

	fmt.Printf("Active hook files: %d (in %s)\n", activeCount, hooksDir)
	fmt.Printf("Total hook files: %d\n", len(entries))
}

// getClaudeConfigDirForHooks returns the Claude config directory for hook operations.
// Checks CLAUDE_CONFIG_DIR env var first, then falls back to ~/.claude
func getClaudeConfigDirForHooks() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// runningUnderClaudeBackground reports whether this hook process descends from
// a Claude Code background worker (`claude bg-spare` / `bg-pty-host`). It walks
// a few ancestors with ps; any failure reads as "not background", so a broken
// ps never silences a real session's hooks.
func runningUnderClaudeBackground() bool {
	var cmds []string
	pid := os.Getppid()
	for i := 0; i < 8 && pid > 1; i++ {
		out, err := exec.Command("ps", "-o", "ppid=,command=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			break
		}
		fields := strings.Fields(strings.TrimSpace(string(out)))
		if len(fields) < 2 {
			break
		}
		cmds = append(cmds, strings.Join(fields[1:], " "))
		next, err := strconv.Atoi(fields[0])
		if err != nil {
			break
		}
		pid = next
	}
	return isClaudeBackgroundAncestry(cmds)
}

// isClaudeBackgroundAncestry is the pure check behind runningUnderClaudeBackground.
func isClaudeBackgroundAncestry(cmds []string) bool {
	for _, c := range cmds {
		if strings.Contains(c, "--bg-spare") || strings.Contains(c, "bg-pty-host") {
			return true
		}
	}
	return false
}
