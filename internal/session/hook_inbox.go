package session

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/millwright-software/agent-desk/internal/logging"
	"github.com/millwright-software/agent-desk/internal/statedb"
)

var hookLog = logging.ForComponent(logging.CompSession)

// HookStatus is the latest Claude Code hook event for an instance, as the
// status loop consumes it.
type HookStatus struct {
	Status    string    // running, waiting, dead
	SessionID string    // Claude session ID
	Event     string    // Hook event name
	UpdatedAt time.Time // When this status was received
}

// HookInbox is the TUI's read side of the hook event table
// (statedb.HookInbox). Refresh() loads the table into memory once per status
// tick; GetHookStatus answers from that snapshot. There is no file watcher
// and no debounce: the status loop already runs on its own ticker, and the
// previous fsnotify watcher only ever fed the same tick.
type HookInbox struct {
	inbox *statedb.HookInbox

	mu       sync.RWMutex
	statuses map[string]*HookStatus
}

// HookInboxPath is where the inbox lives: beside the profiles, not inside
// one, because the writer does not know the profile (see statedb.HookInbox).
func HookInboxPath() string {
	dir, err := GetAgentDeskDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".agent-desk", "hooks.db")
	}
	return filepath.Join(dir, "hooks.db")
}

// NewHookInbox opens the inbox for reading.
func NewHookInbox() (*HookInbox, error) {
	inbox, err := statedb.OpenHookInbox(HookInboxPath())
	if err != nil {
		return nil, err
	}
	return &HookInbox{inbox: inbox, statuses: map[string]*HookStatus{}}, nil
}

// Start imports any status files left by versions before the inbox (one JSON
// file per instance under ~/.agent-desk/hooks/), then takes the first
// snapshot. The import is idempotent: Record keeps the newer row, so a file
// older than what the table already has changes nothing. The files are left
// in place; `agent-desk hooks status` reports them and nothing else reads
// them.
func (h *HookInbox) Start() {
	h.importLegacyFiles(GetHooksDir())
	h.Refresh()
}

// Refresh reloads the snapshot from the table. Called by the status loop
// once per tick, before hook statuses are applied to instances.
func (h *HookInbox) Refresh() {
	all, err := h.inbox.All()
	if err != nil {
		hookLog.Warn("hook_inbox_read_failed", slog.String("error", err.Error()))
		return
	}
	next := make(map[string]*HookStatus, len(all))
	for id, ev := range all {
		next[id] = &HookStatus{Status: ev.Status, SessionID: ev.SessionID, Event: ev.Event, UpdatedAt: ev.At}
	}
	h.mu.Lock()
	h.statuses = next
	h.mu.Unlock()
}

// GetHookStatus returns the latest event for an instance from the current
// snapshot, or nil if none has been seen.
func (h *HookInbox) GetHookStatus(instanceID string) *HookStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.statuses[instanceID]
}

// Prune drops rows for instances that no longer exist and have been quiet
// for a day. Called by the status loop with the live instance IDs.
func (h *HookInbox) Prune(liveInstanceIDs map[string]bool) {
	if n, err := h.inbox.Prune(liveInstanceIDs, time.Now().Add(-24*time.Hour)); err != nil {
		hookLog.Warn("hook_inbox_prune_failed", slog.String("error", err.Error()))
	} else if n > 0 {
		hookLog.Debug("hook_inbox_pruned", slog.Int("rows", n))
	}
}

// Stop closes the table.
func (h *HookInbox) Stop() {
	_ = h.inbox.Close()
}

// legacyHookFile is the pre-inbox status file format.
type legacyHookFile struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
	Event     string `json:"event"`
	Timestamp int64  `json:"ts"`
}

func (h *HookInbox) importLegacyFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	imported := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var f legacyHookFile
		if err := json.Unmarshal(data, &f); err != nil || f.Status == "" {
			continue
		}
		ev := statedb.HookEvent{
			InstanceID: strings.TrimSuffix(e.Name(), ".json"),
			Status:     f.Status, Event: f.Event, SessionID: f.SessionID,
			At: time.Unix(f.Timestamp, 0),
		}
		if err := h.inbox.Record(ev); err == nil {
			imported++
		}
	}
	if imported > 0 {
		hookLog.Info("hook_inbox_imported_legacy_files", slog.Int("files", imported), slog.String("dir", dir))
	}
}

// GetHooksDir returns the path of the pre-inbox status file directory. Kept
// for the legacy import and for `agent-desk hooks status`.
func GetHooksDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".agent-desk", "hooks")
	}
	return filepath.Join(home, ".agent-desk", "hooks")
}
