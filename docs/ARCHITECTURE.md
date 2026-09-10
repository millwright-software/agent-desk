# Agent Desk — Architecture

Developer-oriented map of the codebase. For user-facing docs see `README.md` and
`skills/agent-desk/references/`. This document explains how the pieces fit together
so you (or future-you) can navigate ~77k lines of Go without spelunking.

---

## What it is

A single-binary TUI that manages many terminal-based AI coding sessions (Claude
Code, Gemini CLI, OpenCode, Codex, plus shell/custom tools). Each "session" is a
real **tmux** session running the underlying tool; Agent Desk is the control plane
that creates them, detects their status, and lets you jump between them.

## Process model

Agent Desk is **not** a daemon. It's a foreground TUI process that orchestrates
external state:

```
┌─────────────────────────────────────────────────────────────┐
│  agent-desk (TUI process, Bubble Tea)                         │
│  ┌──────────────┐   polls / watches   ┌────────────────────┐ │
│  │ Home model   │◄───────────────────►│ tmux sessions      │ │
│  │ (internal/ui)│   control-mode pipe │ agentdesk_*        │ │
│  └──────┬───────┘                     └─────────┬──────────┘ │
│         │ read/write                            │ runs       │
│         ▼                                       ▼            │
│  ┌──────────────┐                     ┌────────────────────┐ │
│  │ SQLite state │                     │ claude / gemini /  │ │
│  │ ~/.agent-desk│                     │ opencode / codex   │ │
│  │  (statedb)   │                     └─────────┬──────────┘ │
│  └──────────────┘                               │ hooks      │
│         ▲                                       ▼            │
│         │ multi-process coordination   ┌────────────────────┐ │
│         │ (primary election)           │ Claude Code hooks  │ │
│         └──────────────────────────────│ → status files     │ │
│                                        └────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
        ▲
        │ Unix sockets (optional)
┌───────┴────────────┐
│ MCP socket pool     │  shared MCP processes across sessions
│ (internal/mcppool)  │
└─────────────────────┘
```

Three external state stores back everything: **tmux** (the live sessions), **SQLite**
(`~/.agent-desk/`, durable session/group/status state), and the underlying tools'
own session files on disk (Claude/Gemini/Codex/OpenCode transcripts, used for
session-ID detection, previews, and analytics).

Multiple `agent-desk` processes can run at once (e.g. several terminals). They
coordinate through SQLite **primary election** (`statedb.ElectPrimary`,
`Heartbeat`, `RegisterInstance`, `CleanDeadInstances`) so only one process owns
expensive background duties at a time.

---

## Entry point & command dispatch

`cmd/agent-desk/main.go` is the single entry. With no subcommand it launches the
TUI; otherwise it dispatches a CLI subcommand (`main.go:189`):

| Command | File | Purpose |
|---------|------|---------|
| _(none)_ | `main.go` | Launch the Bubble Tea TUI |
| `add` | `main.go` / `add_test.go` | Create a session (supports `--worktree`, tool flags) |
| `list` / `status` / `remove` | `main.go` | Session table / status / delete |
| `session` | `session_cmd.go` | Fork, send, show, resume, JSON output (1.7k lines) |
| `mcp` | `mcp_cmd.go` | Attach/detach/list MCP servers per session |
| `mcp-proxy` | `mcp_proxy_cmd.go` | The pooled-MCP reconnecting proxy process |
| `group` | `group_cmd.go` | Group create/move/reorder |
| `worktree` / `wt` | `worktree_cmd.go` | Worktree add / finish (merge+cleanup) / cleanup |
| `try` | `try_cmd.go` | Quick throwaway session |
| `hook-handler` | `hook_handler.go` | Receives Claude Code lifecycle hook callbacks |
| `hooks` | `main.go` | Install/uninstall/status for Claude hooks |
| `profile` / `update` / `uninstall` | `main.go` | Profile switch, self-update, uninstall |

A leading `-p <profile>` flag is extracted before dispatch (`extractProfileFlag`)
so every command is profile-scoped.

---

## Package map (`internal/`)

| Package | Lines¹ | Responsibility |
|---------|-------:|----------------|
| `session/` | ~28k | **Core domain.** Instance model, status detection, per-tool command building, storage, groups, MCP catalog, global search, analytics, config |
| `ui/` | ~22k | **The TUI.** Bubble Tea `Home` model + all dialogs/panels |
| `tmux/` | ~10k | tmux integration: session lifecycle, control-mode pipe, PTY, prompt/status detection |
| `mcppool/` | ~1.8k | MCP socket pool: shared MCP processes over Unix sockets + reconnecting proxy |
| `statedb/` | ~1.5k | SQLite persistence + multi-process coordination |
| `git/` | ~0.5k | Worktree creation, branch templates |
| `logging/` | ~0.9k | Structured logging, ring buffer, pprof, slog compat |
| `update/` | ~0.5k | Self-update (GitHub releases / Homebrew aware) |
| `experiments/`, `profile/`, `platform/`, `clipboard/` | small | Feature flags, profile detection, OS specifics, clipboard |

¹ Approximate, includes tests.

---

## Core data model: `Instance`

`session/instance.go` (3.5k lines) is the heart. An `Instance` is one
agent/shell session. Key fields (`instance.go:42`):

- **Identity/layout:** `ID`, `Title`, `ProjectPath`, `GroupPath`, `Order`
- **Hierarchy:** `ParentSessionID` (sub-sessions/forks), worktree fields
  (`WorktreePath`, `WorktreeRepoRoot`, `WorktreeBranch`)
- **Tool:** `Tool` + `Command` + optional `Wrapper` (`{command}` placeholder),
  `ToolOptionsJSON` (tool-specific launch options)
- **Per-tool session IDs:** `ClaudeSessionID`, `GeminiSessionID`,
  `OpenCodeSessionID`, `CodexSessionID` — detected after launch so sessions can be
  resumed/forked
- **Status:** `Status` + hook fields (`hookStatus`, `hookSessionID`,
  `hookLastUpdate`)
- **Context:** `LatestPrompt` (auto-extracted) / `ContextNote` (manual, wins)

### Status lifecycle

```
StatusStarting → StatusRunning ⇄ StatusWaiting → StatusIdle
                      │
                      └──────────► StatusError
```

| Status | Meaning |
|--------|---------|
| `starting` | tmux session being created (grace period; prevents error flash) |
| `running` | agent actively working |
| `waiting` | needs user input |
| `idle` | ready for commands |
| `error` | tmux session gone / crashed |

`Instance` is mutated by a **background goroutine** and read by the **UI
goroutine**, so status/tool access goes through a `sync.RWMutex` via
`GetStatusThreadSafe`/`SetStatusThreadSafe` etc. (`instance.go:136`).

---

## Status detection (the interesting part)

Two complementary mechanisms, hybrid by design:

**1. tmux content polling** — `Instance.UpdateStatus()` (`instance.go:1338`)
captures the tmux pane and runs it through `tmux/detector.go` +
`tmux/patterns.go`. `PromptDetector` has per-tool logic
(`hasClaudePrompt`, `hasGeminiPrompt`, `hasShellPrompt`,
`hasOpencodeBusyIndicator`) over ANSI-stripped content to decide
running/waiting/idle. Busy-spinner regexes detect active work.

**2. Claude Code lifecycle hooks** — when installed, Claude calls back into
`agent-desk hook-handler` (`cmd/.../hook_handler.go`) which writes status files.
`session/hook_watcher.go` (`StatusFileWatcher`, fsnotify-based) picks them up and
calls `Instance.UpdateHookStatus()` for **instant** green/yellow/gray transitions
with no polling latency. Sessions with live hooks largely skip content polling.

Polling is heavily optimized to keep the TUI responsive (see `Home` comments,
`ui/home.go:191+`):
- **Round-robin batching** — only 5–10 sessions updated per tick, not all.
- **Tiered polling** — idle sessions with no `window_activity` change get cheap
  checks (`lastIdleCheck`, `lastKnownActivity`).
- **Background worker goroutine** — status updates run off the UI thread
  (`backgroundStatusUpdate`, `home.go:1626`), fed via `statusTrigger`.
- **Output-driven updates** — tmux control-mode `%output` events flow through
  `tmux/pipemanager.go` into a capped worker pool (`logUpdateChan`), debounced.

---

## The TUI (`internal/ui`)

Standard Bubble Tea **Elm architecture**: `Home.Init/Update/View`. `Home`
(`ui/home.go:122`, 8.4k lines) is the root model holding:

- **Data:** `instances` + `instanceByID` (O(1) lookup), `groupTree`, `flatItems`
  (flattened tree for cursor navigation), all under `instancesMu`.
- **Components:** one struct per dialog/panel — `search`, `globalSearch`,
  `newDialog`, `forkDialog`, `mcpDialog`, `settingsPanel`, `analyticsPanel`,
  `setupWizard`, `worktreeFinishDialog`, etc.
- **Async caches:** `View()` must be pure (no blocking I/O), so previews and
  analytics are fetched on goroutines into TTL caches (`previewCache`,
  `analyticsCache`) and previews are **debounced 150ms** during rapid j/k nav.

The main screen is the dual-column layout (`renderDualColumnLayout`): a session
sidebar (configurable width %, `sidebarWidthPercent`, 15–50) plus a preview pane.

External changes (CLI edits, other processes) are caught by `StorageWatcher`
(`ui/storage_watcher.go`, fsnotify) for auto-reload; `ThemeWatcher` syncs the TUI
to OS dark/light mode when `theme="system"`.

Rendering/colors live in `ui/styles.go`; color profile (TrueColor/256/ANSI) is
chosen in `main.go:initColorProfile` and overridable via `AGENTDESK_COLOR`.

---

## Persistence (`internal/statedb`)

SQLite at `~/.agent-desk/` (per profile). `statedb.go` is a thin typed layer over
`database/sql` (driver: `modernc.org/sqlite`, pure-Go, no cgo). It stores
instances, groups, and a status table, plus:

- **Migrations** — `migrate.go`, run on `Open`.
- **Multi-process coordination** — `RegisterInstance`/`Heartbeat`/`ElectPrimary`/
  `ResignPrimary`/`CleanDeadInstances`. Each running TUI registers and heartbeats;
  one is elected primary to avoid duplicated background work.
- **Change detection** — `Touch`/`LastModified` let watchers detect external edits.

`session/storage.go` is the higher-level Storage API the UI talks to; it maps
`InstanceRow` ⇄ `Instance`.

---

## tmux integration (`internal/tmux`)

`tmux.go` (3.5k lines) wraps the tmux CLI for session create/attach/send/kill and
status-bar injection (sessions are prefixed `agentdesk_*` so they never collide
with the user's own tmux). Supporting files:

- `controlpipe.go` + `pipemanager.go` — tmux **control mode** for event-driven
  window/output notifications instead of pure polling.
- `pty.go` — PTY handling (`creack/pty`).
- `detector.go` / `patterns.go` — prompt & busy-state detection (see above).
- `title_detection.go` — detect the AI tool running in a pane.

---

## MCP socket pool (`internal/mcppool`)

When `pool_all = true`, MCP servers run **once** and are shared across all
sessions via Unix sockets instead of one process per session (the README cites
85–90% memory reduction). `pool_simple.go` manages server lifecycle
(`ServerStatus`: stopped/starting/running/failed/permanently_failed in
`types.go`); `socket_proxy.go` is a reconnecting proxy that recovers from MCP
crashes in ~3s; `http_pool.go`/`http_server.go` handle HTTP-transport MCPs. The
`mcp-proxy` subcommand runs the proxy process.

---

## Multi-tool support

Per-tool launch and detection logic lives in `session/`:
`claude.go` + `claude_hooks.go`, `gemini.go`, `opencode_*`, and the Codex paths in
`instance.go`. Command construction is centralized in `Instance.build*Command`
methods (`buildClaudeCommand`, `buildGeminiCommand`, `buildOpenCodeCommand`,
`buildCodexCommand`, `buildGenericCommand`). New custom tools are configured via
`[tools.*]` in config without code changes.

## Configuration (`internal/session/userconfig.go`)

TOML at `~/.agent-desk/config.toml`, parsed into a `UserConfig` struct and cached
via `LoadUserConfig()`. Sections include `[claude]` (e.g. `hooks_enabled`),
`[tmux]` (`inject_status_line`), `[worktree]` (`default_location`), `[tools.*]`
(custom tools), MCP definitions, and the `pool_all` socket-pool toggle. See
`skills/agent-desk/references/config-reference.md` for the full surface.

---

## Build & test

```bash
make build        # or: go build ./cmd/agent-desk/
make test         # go test ./...
make lint
go test ./internal/ui/... ./internal/session/...   # the two heaviest packages
```

Release tooling: `.goreleaser.yml`, `install.sh`/`uninstall.sh`, Homebrew tap.
Pre-commit hooks via `lefthook.yml`.

---

## Concurrency model — read this before editing the TUI

The recurring hazard is the **UI goroutine vs. background workers** boundary:

- `Instance` status/tool fields: always use the `*ThreadSafe` accessors.
- `Home.instances`/`instanceByID`: guard with `instancesMu`.
- Caches (`previewCache`, `analyticsCache`, `worktreeDirtyCache`, `lastLogActivity`)
  each have their own mutex — `View()` reads them but never blocks on I/O.
- `reloadVersion` (under `reloadMu`) invalidates stale background saves after a
  reload — bump it when you add new async write paths.

---

## Project lineage

Agent Desk is a private rename-fork of
[`asheshgoplani/agent-deck`](https://github.com/asheshgoplani/agent-deck) (the
`upstream` remote), branched at ~v0.17. It intentionally stays lighter than
current upstream (no cost dashboard, web mode, Docker sandbox, OpenClaw, or remote
SSH — those landed upstream after the fork point). See the local `td.md` board for the upstream
sync notes.
