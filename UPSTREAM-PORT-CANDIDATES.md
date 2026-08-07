# Upstream port candidates (agent-deck → agent-desk)

Survey run 2026-07-24. Fork point (merge-base): `94a4aade` (2026-02-15).
Upstream `asheshgoplani/agent-deck`, ~2046 commits ahead. Each item was
cross-checked against our fork's actual code. **Deferred — do the local
preview/UX work first.**

Key fork fact: we ship `AllowMultiple=true` (concurrent TUIs/CLI), which makes
the full-table-sweep saves an ACTIVE data-loss risk, not theoretical.

## Tier 1 — clean, low-risk, high-value (do first)
- `61b90dde` — refuse empty-payload `SaveInstances` sweep (an empty save runs
  unconditional `DELETE FROM instances` = whole table wiped). Ours: statedb.go:257. Clean.
- `36289703` — EOF-anchored last-assistant extraction survives `/compact`
  (1 MB `bufio.Scanner` stops at compact's multi-MB single-line records → stale
  output forever). Ours: instance.go parseClaudeLastAssistantMessage. Clean.
- `99e3674e` — `ElectPrimary` reclaims dead primary via PID `kill -0` (stale
  is_primary → refuses to launch until pkill). Ours: statedb.go:523. Clean.
- `b1dbda25` — `Exists()` bounded timeout; don't treat any tmux hiccup as
  "session gone" (false red flips). Ours: tmux.go:1017. Clean.
- `4d19834d` — `safeGo()` panic-recovery wrapper on ~6 unrecovered goroutines
  (one panic kills the TUI). Clean.
- `77b9502d` — "no server running" via `ExitError.Stderr` not `err.Error()`
  (branch never matches with cmd.Output → returns error vs empty list). Clean.

## Tier 2 — high value, needs adaptation
- Data-loss cluster (port together, shared root cause = full-table sweep):
  - `d6f0d7bb` — upsert-only routine saves; stop `DELETE ... WHERE id NOT IN
    (stale snapshot)` deleting other processes' sessions. statedb.go:249/SaveWithGroups.
  - `69d11f75` — reject group rename onto existing sibling path (silent
    overwrite → orphaned group's sessions swept = permanent loss). groups.go:720.
  - `0c043099` — additive `SaveGroups` upsert (empty groups vanish on
    replace-all). statedb.go:363. Reconcile with our "hide empty My Sessions".
- `0879ef3e` — background-work (run_in_background / awaited agent) misread as
  "finished" → premature done notification. No background-work concept in fork.
- `ed062900` — auth/401/socket error banners classified as waiting (orange)
  not error; logged-out session shows waiting forever. detector.go has no
  error-banner strings.
- `9954dfb1` — 30–50s UI freeze when tmux server dead but socket stale; add
  `IsServerAlive()` short-circuit + timeouts on un-timeouted tmux calls.
- `71466cb2` (+`1958b276`) — emoji/keycap cell-width undercount → whole-list
  layout drift. Adds `internal/ui/cellwidth.go`; swap 13 `runewidth.*` sites.
- `33673cfa` — stale `IsLastInGroup` → wrong tree connector (`├─` vs `└─`) on
  last visible row under status filter. Ours: rebuildFlatItems filters after
  Flatten() bakes the flag (home.go:840-880). Confirmed live.

## Tier 3 — lower / opportunistic / needs-verify
- `69089c90` — backup state.db before destructive writes + refuse config
  section-drop ([mcps] wipe). userconfig.go has zero guards.
- `16db87ef` — group create/rename/move survive save-abort reload race (mirror
  our pendingTitleChanges pattern for groups).
- `1145387c` (part 2) — external-change detection via `GetUpdatedAt` not
  `GetFileMtime` (WAL-blind os.Stat). home.go:1198/2680. Lower severity (our
  StorageWatcher already uses metadata LastModified).
- `1941c645` — resolve Claude transcript by session-id when project-dir name
  mismatches (WSL). Low priority for macOS.
- `86749a5f` — dialog width overflow on narrow terminals (needs cellwidth.go).
- `fe7e9c7e` (bug 1) — preview pane clipped at narrow widths / no min clamp.
  Ours: home.go:6073-6074 no floor.
- `5e2c340a` — default-group delete error swallowed by viewport clamp
  (needs-verify our layout).
- `a9ea185d` — sticky session-id on stale tool_data saves (design item, tie to
  the data-loss cluster). `08e0efb7` — nested `add -g work/bar` spurious group.

## Explicitly skip
- `internal/sessionstatus` package adoption — internal reorg for web parity we
  don't have; hook-mapping already inline in our instance.go fast path.
- `f4d7d9f9` Codex `›` prompt — we deliberately minimized Codex.
- All archive / notify-daemon / transition_daemon fixes — that architecture
  doesn't exist in our fork.
