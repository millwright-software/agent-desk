# Session Status Icons — Diagnosis and Fix Plan

**Status:** Planning
**Date:** 2026-02-26
**Branch:** TBD

---

## Problem

Session status icons don't reliably reflect actual session state. Users observe icons that appear stuck, wrong, or slow to update.

## Current Implementation

### Five Status States

| Status | Icon | Color | Meaning |
|--------|------|-------|---------|
| `StatusRunning` | `●` | Green | Claude actively processing |
| `StatusWaiting` | `◐` | Yellow | Output waiting for user |
| `StatusIdle` | `○` | Gray | Output acknowledged or user attached |
| `StatusError` | `✕` | Red | Session doesn't exist in tmux |
| `StatusStarting` | `⟳` | Yellow | Session initializing (1.5s grace) |

**Defined in:** `internal/session/instance.go:29-37`

### Two-Tier Detection System

#### Tier 1: Hook-Based (Claude Code sessions only)

**Files:** `internal/session/hook_watcher.go:20-144`, `internal/session/instance.go:1613-1670`

- Claude Code writes status files to `~/.agent-desk/hooks/`
- `StatusFileWatcher` monitors these files via filesystem watching
- Hook status values: `"running"`, `"waiting"`, `"dead"`
- **2-minute staleness timeout:** If no hook update in 2 min, falls through to tmux polling
- Quality gate: Only accepts hook session IDs with conversation data (prevents zombie IDs)

**Status mapping from hooks:**
- `"running"` → StatusRunning (reset acknowledged flag)
- `"waiting"` + acknowledged → StatusIdle
- `"waiting"` + NOT acknowledged → StatusWaiting
- `"dead"` → StatusError

#### Tier 2: Tmux Polling (non-Claude tools, stale hook fallback)

**File:** `internal/tmux/tmux.go:1641-2044`

Multi-stage detection:

1. **Quick checks** (lines 1647-1683): Session existence, pane title Braille spinner fast path
2. **Activity timestamp** (lines 1685-1815): `GetWindowActivity()` (~4ms), only proceed to expensive `CapturePane()` if activity changed
3. **Busy indicator detection**: Braille spinners (⣾⣽⣻⢿⡿⣟⣯⣷) or "ctrl+c to interrupt" pattern
4. **Prompt detection**: Looks for `❯` prompt indicator
5. **Spike detection** (lines 1857-1950): 2+ activity changes within 1-second window, confirmed with content check
6. **Acknowledgment logic** (lines 1778-1854): Prompt + acknowledged → idle; Prompt + not acknowledged → waiting
7. **Startup window** (lines 1804-1811): First 10 seconds → `"starting"` to avoid premature yellow flash

### Status Determination Priority

| Priority | Condition | Result |
|----------|-----------|--------|
| 1 | Session doesn't exist in tmux | StatusError |
| 2 | Claude + fresh hook data (<2min) | Hook-determined |
| 3 | Pane title has Braille spinner | StatusRunning |
| 4 | Busy indicator visible | StatusRunning |
| 5 | Activity spike + busy check | StatusRunning |
| 6 | Prompt + acknowledged | StatusIdle |
| 7 | Prompt + not acknowledged | StatusWaiting |
| 8 | In startup window (<10s) | StatusStarting |
| 9 | No busy/prompt, acknowledged | StatusIdle |
| 10 | Default fallback | StatusWaiting |

### Update Cycle

**File:** `internal/ui/home.go:1555-1786`

- Background ticker: every **2 seconds**
- Up to 10 concurrent `UpdateStatus()` calls via errgroup
- Idle session optimization: skip if no output in 5+ seconds
- Ghost sessions (not in tmux): rechecked only every 30 seconds
- SQLite sync for multi-instance coordination

### Rendering

**File:** `internal/ui/home.go:6742-6859`

- `renderSessionItem()` reads status via `inst.GetStatusThreadSafe()`
- Maps status → icon + lipgloss style
- Format: `[indent][selection][tree][status] [title] [worktree]`

## Diagnosed Issues

### 1. Whimsical Word Detection Gap (HIGH IMPACT)

**File:** `internal/tmux/status_fixes_test.go:7-60`

Only "Thinking" and "Connecting" trigger busy detection. Claude's 90+ other status words ("Analyzing", "Brainstorming", "Pondering", etc.) are missed entirely.

**Impact:** Sessions show YELLOW (waiting) instead of GREEN (running) while Claude is actively working with a non-detected status word. This is likely the most visible symptom users notice.

**Fix:** Expand busy indicator detection to cover all Claude whimsical words, or detect the pattern generically (capitalized word + ellipsis/spinner in same region).

### 2. Spinner Staleness — No Timeout (MEDIUM IMPACT)

**File:** `internal/tmux/status_fixes_test.go:121-172`

If Claude crashes or hangs with spinner visible in pane, status stays GREEN forever. No mechanism to detect the spinner is stale.

**Impact:** Dead sessions appear perpetually "running."

**Fix:** Track last content hash change time. If spinner visible but content unchanged for >30s, stop trusting the spinner and fall through to other detection.

### 3. Hook 2-Minute Timeout Window (MEDIUM IMPACT)

**File:** `internal/session/instance.go:1400`

When Claude stops sending hook updates (crash, hang, network issue), there's a full 2-minute window where status is stale before falling back to tmux polling.

**Impact:** Status lags reality by up to 2 minutes after a crash.

**Fix:** Consider reducing timeout to 30-60 seconds, or adding a secondary signal (tmux existence check) as an early exit from hook trust.

### 4. Acknowledge Race Condition (LOW IMPACT)

**File:** `internal/tmux/status_fixes_test.go:334-350`

User attaches at exact moment new output arrives → misses "waiting" state, shows idle.

**Impact:** Rare but theoretically observable.

**Fix:** 100ms grace period after acknowledge before accepting new content changes.

### 5. Progress Bar Flicker (LOW IMPACT)

**File:** `internal/tmux/status_fixes_test.go:179-226`

Dynamic progress bars cause content hash changes → false GREEN flickers.

**Impact:** Intermittent green flashes during progress bar updates.

**Fix:** Better normalization of progress bar content before hashing.

## Key Files

| File | Purpose | Key Lines |
|------|---------|-----------|
| `internal/session/instance.go` | Status constants + UpdateStatus | 29-37, 1336-1481 |
| `internal/session/instance.go` | Hook status integration | 1613-1670 |
| `internal/tmux/tmux.go` | GetStatus tmux polling | 1641-2044 |
| `internal/session/hook_watcher.go` | StatusFileWatcher | 20-144 |
| `internal/ui/home.go` | Background status loop | 1555-1786 |
| `internal/ui/home.go` | renderSessionItem | 6742-6859 |
| `internal/ui/styles.go` | StatusIndicator styling | 560-581 |
| `internal/tmux/status_fixes_test.go` | Documented bugs & proposed fixes | Full file |

## Recommended Fix Order

1. **Whimsical word detection** — Highest user-visible impact, straightforward fix
2. **Spinner staleness timeout** — Prevents stuck-green on crashed sessions
3. **Reduce hook timeout** — 2 min → 30-60s for faster crash recovery
4. **Acknowledge race** — Nice-to-have, low priority
5. **Progress bar normalization** — Nice-to-have, low priority

## Open Questions

- Should whimsical words be detected via a hardcoded list or a pattern (e.g., regex for capitalized word near spinner)?
- Is 30s the right threshold for spinner staleness, or should it be configurable?
- Does reducing hook timeout to 30s risk false fallbacks during normal operation (e.g., slow Claude responses)?
