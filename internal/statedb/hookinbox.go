package statedb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// HookInbox is the one table Claude Code hook events land in. The hook
// handler (a short-lived `agent-desk hook-handler` process spawned by Claude
// Code) upserts one row per agent-desk instance; the TUI reads the whole
// table once per status tick. It replaces a directory of one JSON file per
// instance plus an fsnotify watcher — a side channel the rest of the state
// never saw.
//
// ⚠️ It is its OWN database file, not a table in the profile's state.db,
// because the handler does not know which profile spawned the session (only
// AGENTDESK_INSTANCE_ID is in its environment), and guessing would be a worse
// tunnel than the one being removed. Instance IDs are globally unique, so a
// profile-agnostic inbox keyed by them is exact. It also keeps the hook
// writers off state.db's lock entirely.
//
// Contention, measured with this driver on 2026-10-03 (one fresh process per
// write, WAL, busy_timeout 5s): 30 processes × 40 writes, zero errors, p50
// 0.5 ms, max 10 ms; 100 processes × 30 writes, zero errors, max 98 ms. The
// hook is async in Claude Code, so none of that is on the user's path.
type HookInbox struct {
	db *sql.DB
}

// HookEvent is one row: the latest hook event seen for an instance.
type HookEvent struct {
	InstanceID string
	Status     string // running, waiting, dead
	Event      string // the Claude Code hook event name
	SessionID  string // Claude session ID carried by the event
	At         time.Time
}

// OpenHookInbox opens (creating if needed) the inbox at path.
func OpenHookInbox(path string) (*HookInbox, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("hookinbox: mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("hookinbox: open: %w", err)
	}
	for _, p := range []string{
		"PRAGMA journal_mode=WAL",   // readers never block the writer
		"PRAGMA busy_timeout=5000",  // a burst of handlers queues, never fails
		"PRAGMA synchronous=NORMAL", // a lost last event on power loss is fine; the next one replaces it
	} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("hookinbox: %s: %w", p, err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS hook_events (
		instance_id TEXT PRIMARY KEY,
		status      TEXT NOT NULL,
		event       TEXT NOT NULL,
		session_id  TEXT NOT NULL DEFAULT '',
		ts          INTEGER NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("hookinbox: schema: %w", err)
	}
	return &HookInbox{db: db}, nil
}

// Close releases the connection. Safe on nil.
func (h *HookInbox) Close() error {
	if h == nil || h.db == nil {
		return nil
	}
	return h.db.Close()
}

// Record upserts the latest event for an instance. A row older than ev.At
// is replaced; a newer one is kept, so replaying stale events (the legacy
// file import, or two handlers racing) can never move status backwards.
func (h *HookInbox) Record(ev HookEvent) error {
	_, err := h.db.Exec(`INSERT INTO hook_events (instance_id, status, event, session_id, ts)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(instance_id) DO UPDATE SET
			status = excluded.status, event = excluded.event,
			session_id = excluded.session_id, ts = excluded.ts
		WHERE excluded.ts >= hook_events.ts`,
		ev.InstanceID, ev.Status, ev.Event, ev.SessionID, ev.At.Unix())
	if err != nil {
		return fmt.Errorf("hookinbox: record: %w", err)
	}
	return nil
}

// All returns every row, keyed by instance ID.
func (h *HookInbox) All() (map[string]HookEvent, error) {
	rows, err := h.db.Query(`SELECT instance_id, status, event, session_id, ts FROM hook_events`)
	if err != nil {
		return nil, fmt.Errorf("hookinbox: all: %w", err)
	}
	defer rows.Close()
	out := map[string]HookEvent{}
	for rows.Next() {
		var ev HookEvent
		var ts int64
		if err := rows.Scan(&ev.InstanceID, &ev.Status, &ev.Event, &ev.SessionID, &ts); err != nil {
			return nil, fmt.Errorf("hookinbox: scan: %w", err)
		}
		ev.At = time.Unix(ts, 0)
		out[ev.InstanceID] = ev
	}
	return out, rows.Err()
}

// Count is the number of rows, for `agent-desk hooks status`.
func (h *HookInbox) Count() (int, error) {
	var n int
	err := h.db.QueryRow(`SELECT COUNT(*) FROM hook_events`).Scan(&n)
	return n, err
}

// Prune deletes rows whose instance is not in keep and whose last event is
// older than olderThan. Instances come and go; their last "dead" event has no
// reader once the instance is deleted.
func (h *HookInbox) Prune(keep map[string]bool, olderThan time.Time) (int, error) {
	all, err := h.All()
	if err != nil {
		return 0, err
	}
	pruned := 0
	for id, ev := range all {
		if keep[id] || ev.At.After(olderThan) {
			continue
		}
		if _, err := h.db.Exec(`DELETE FROM hook_events WHERE instance_id = ?`, id); err != nil {
			return pruned, fmt.Errorf("hookinbox: prune: %w", err)
		}
		pruned++
	}
	return pruned, nil
}
