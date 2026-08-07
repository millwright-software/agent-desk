# Agent Desk

TUI session manager for AI coding tools (Claude Code, Gemini, OpenCode, Codex).

**Priorities:** See `td.md`

**Requires:** Go 1.22+, tmux

---

## Stack

Go, Bubble Tea (TUI framework), tmux, SQLite (state persistence), TOML (config).

## Build

```bash
go build ./cmd/agent-desk/
go test ./internal/ui/... ./internal/session/...
```

## Key Paths

- `cmd/agent-desk/` — Main entry point
- `internal/ui/` — TUI components (home, settings, search, help)
- `internal/session/` — Session management, config, storage
- `internal/tmux/` — tmux integration

## Architecture Notes

- **Config:** `internal/session/userconfig.go` — TOML-based `UserConfig` struct, cached with `LoadUserConfig()`
- **UI State:** Persisted to SQLite via `saveUIState()`/`loadUIState()` in `home.go`
- **Settings Panel:** `settings_panel.go` — enum-indexed settings with `settingsCount`, `cursorToLine` scroll mapping
- **Dual-column layout:** `renderDualColumnLayout()` in `home.go` — sidebar + preview pane
