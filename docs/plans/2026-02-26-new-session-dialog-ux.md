# New Session Dialog — Directory Picker UX Improvement

**Status:** Planning
**Date:** 2026-02-26
**Branch:** TBD

---

## Problem

The New Session dialog's directory picker is clunky for navigating project trees. Tab-completing nonexistent paths silently returns nothing (no error, no feedback). Users with a known collection of handoff nodes just want to pick a project, not type filesystem paths character by character.

## Current Implementation

### Two Modes

**List Mode (default when suggestions exist):**
- Shows up to 5 recent paths with scroll indicators
- Arrow keys navigate, Enter/Tab accepts and advances focus
- Typing any printable character switches to text mode
- Populated from all previous session `ProjectPath` values, sorted by most recently accessed

**Text Mode (free-form input):**
- Tab triggers `GetDirectoryCompletions()` with cycling via `CompletionCycler`
- Supports tilde expansion (`~/path`)
- Esc returns to list mode if suggestions exist

### Tab Completion Logic

**File:** `internal/session/utils.go:33-94`

```go
func GetDirectoryCompletions(input string) ([]string, error)
```

- Handles tilde-prefixed paths
- Scans parent directory for subdirectories matching input prefix
- Returns `nil, nil` for invalid parent dirs (no error surfaced to user)
- Only scans immediate children — no recursive or fuzzy matching

### Path Suggestion Population

**File:** `internal/ui/home.go:3892-3940`

- Collects unique `ProjectPath` values from all `Instance` objects
- Tracks `lastAccessedAt` per path
- Sorts by most recent first
- Called when dialog opens via `h.newDialog.SetPathSuggestions(paths)`

### Per-Group Defaults

**File:** `internal/session/groups.go:38-45`

- `Group.DefaultPath` stores most recent project path for sessions in that group
- Retrieved via `getDefaultPathForGroup()` in `home.go:1542-1552`

### Validation

**File:** `internal/ui/newdialog.go:334-366`

- Checks name is non-empty and within `MaxNameLength`
- Checks path is non-empty but does NOT validate directory existence
- Malformed path recovery (lines 234-242): detects `/some/path~/actual/path` from suggestion appending bugs

### Handoff Framework Awareness: NONE

- No references to `.handoff.yaml` in codebase
- No node type or identity detection
- No parent/child relationship awareness
- Only existing prior art: MCP parent lookup walks dirs for `.mcp.json` (`internal/session/claude.go:186-209`)

## Key Files

| File | Purpose | Key Lines |
|------|---------|-----------|
| `internal/ui/newdialog.go` | Dialog implementation | 16-55 (struct), 502-540 (Tab handling), 824-877 (rendering) |
| `internal/ui/newdialog_test.go` | Dialog tests | 186-325 (path list mode), 889-915 (text input guards) |
| `internal/session/utils.go` | Path completion | 33-94 (GetDirectoryCompletions), 96-127 (CompletionCycler) |
| `internal/ui/home.go` | Path suggestion population | 3892-3940 (collect paths), 1542-1552 (group defaults) |
| `internal/session/groups.go` | Group/path structure | 38-45 (Group struct with DefaultPath) |
| `internal/session/claude.go` | MCP parent lookup (reference pattern) | 186-209 (walks parent dirs for .mcp.json) |

## Design Direction

### Option A: Handoff-Node-Aware Project Picker

Make agent-desk aware of the handoff framework's `.handoff.yaml` files:

1. **Node Discovery:** Scan configured root paths (the directories where the user keeps their repos) for `.handoff.yaml` files
2. **Present as structured list:** Show nodes by name with hierarchy context instead of raw paths
3. **Config source:** Could be a new section in agent-desk's TOML config specifying root scan paths, or auto-discover from recent session paths

**Pros:** Natural "pick a project" UX, leverages existing framework structure
**Cons:** Couples agent-desk to handoff framework, needs config for scan roots

### Option B: Enhanced Filesystem Picker

Keep it filesystem-based but improve the UX:

1. **Fuzzy matching:** Instead of prefix-only, support fuzzy/substring matching on directory names
2. **Deeper scanning:** Scan 2-3 levels deep instead of just immediate children
3. **Better feedback:** Show "no matches" message instead of silent nothing
4. **Bookmarks:** Let users pin favorite paths in config

**Pros:** Framework-agnostic, simpler implementation
**Cons:** Still path-oriented rather than project-oriented

### Option C: Hybrid

- Default to handoff node list if nodes are discoverable
- Fall back to enhanced filesystem picker otherwise
- Keep text mode as escape hatch for arbitrary paths

## Open Questions

- Where should scan root paths be configured? TOML config? Auto-discovered?
- Should node discovery be recursive (find all `.handoff.yaml` in tree) or configured explicitly?
- How deep should filesystem scanning go for non-handoff paths?
- Should the picker show node metadata (type, parent) or just name + path?
