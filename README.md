<div align="center">

<img src="site/logo.svg" alt="Agent Desk" width="96">

# Agent Desk

**Terminal command center for AI coding agents.**

Run and switch between Claude Code, Gemini, Codex, and other terminal AI tools —
each in its own tmux session — from one keyboard-driven view.

[![Release](https://img.shields.io/github/v/release/millwright-software/agent-desk?style=flat-square&color=e0af68&labelColor=1a1b26)](https://github.com/millwright-software/agent-desk/releases)
[![License](https://img.shields.io/badge/License-MIT-9ece6a?style=flat-square&labelColor=1a1b26)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&labelColor=1a1b26)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20WSL-7aa2f7?style=flat-square&labelColor=1a1b26)](#install)

</div>

> **A Millwright Software fork of [`asheshgoplani/agent-deck`](https://github.com/asheshgoplani/agent-deck)** by Ashesh Goplani.
> Maintained independently, distributed under the original MIT license (see [`LICENSE`](LICENSE)); upstream
> authorship is preserved in the commit history.

## About this fork

A personal, stripped-down fork, tuned for how I work.

**What's gone:** the conductor orchestration layer and the web/dashboard surface — just Claude Code and GitHub
Copilot CLI in tmux.

**What's sharper** — the controls I use every day:

- **Name-forward path picker** — folder name first, `~` collapse, recent-project stars
- **Model + mode on every row** — see each session's model and auto/manual mode at a glance
- **Manual flags** (`u`) — mark a session unread or parked to come back to
- **Per-session color schemes** (`c`) — tint the tmux window and preview per session
- **Worktree-manager overlay** — clear orphaned worktrees without dropping to the CLI

Upstream is bigger, more featured, and actively maintained — if you want the full toolkit, use it.

---

## What it does

Running Claude Code on a dozen projects, plus a couple of Gemini and Codex sessions? Managing that as a wall of
terminal tabs gets messy fast — hard to see what's running, what's waiting on you, and where you left off.

Agent Desk puts every agent session in one place:

- **See status at a glance** — running, waiting, idle, or errored, for every session
- **Switch in a keystroke** — jump to any session instantly; attach/detach without losing state
- **Stay organized** — groups, fuzzy search, manual flags, and git worktrees
- **Fork Claude conversations** — branch a session with full context to try a different approach

It's tmux underneath, with AI-aware status detection and session management layered on top.

## Install

**Works on:** macOS, Linux, Windows (WSL). Requires `tmux`.

**Quick install** (prebuilt binary — macOS arm64/Intel, Linux amd64/arm64):

```bash
curl -fsSL https://raw.githubusercontent.com/millwright-software/agent-desk/main/install.sh | bash
```

**With Go** (requires Go 1.24+):

```bash
go install github.com/millwright-software/agent-desk/cmd/agent-desk@latest
```

**From source:**

```bash
git clone https://github.com/millwright-software/agent-desk.git
cd agent-desk
make install
```

Then run `agent-desk`. To update, re-run the installer (or `go install …@latest` / `git pull && make install`).

## Quick start

```bash
agent-desk                        # Launch the TUI
agent-desk add . -c claude        # Add the current dir as a Claude session
agent-desk session fork my-proj   # Fork a Claude session
agent-desk mcp attach my-proj exa # Attach an MCP server to a session
```

### Key shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Attach to session |
| `n` | New session |
| `f` / `F` | Fork (quick / dialog) |
| `u` | Cycle manual flag: unread → parked → normal |
| `M` | MCP Manager |
| `/` / `G` | Search / global search across all Claude conversations |
| `Shift+J` / `Shift+K` | Reorder session down / up |
| `r` | Restart session |
| `d` | Delete session |
| `?` | Full help |

See the [TUI Reference](skills/agent-desk/references/tui-reference.md) for every shortcut and the
[CLI Reference](skills/agent-desk/references/cli-reference.md) for every command.

## Features

### Status detection

Agent Desk polls each session and infers what its agent is doing:

| Status | Symbol | Meaning |
|--------|--------|---------|
| **Running** | `●` green | Actively working |
| **Waiting** | `◐` yellow | Needs your input |
| **Idle** | `○` gray | Ready for commands |
| **Error** | `✕` red | Something went wrong |

For Claude, detection uses the session hook and transcript for fast, accurate state (including the current model).
Other tools fall back to output-pattern and tmux-activity detection.

### Fork sessions

Fork any Claude conversation instantly — each fork inherits the full history, so you can explore a different
approach without losing your place. Press `f` to fork, `F` to name/group it. Fork forks as deep as you like.

### MCP manager

Attach MCP servers without hand-editing config files. Press `M`, `Space` to toggle a server, `Tab` to cycle scope
(local / global). Define your servers once in `~/.agent-desk/config.toml`; toggle them per session. Agent Desk
handles the restart. See the [Configuration Reference](skills/agent-desk/references/config-reference.md).

**MCP socket pool** — with `pool_all = true`, MCP processes are shared across sessions over Unix sockets, cutting
MCP memory ~85–90% and auto-recovering from crashes in a few seconds via a reconnecting proxy.

### Git worktrees

Run multiple agents against the same repo without conflicts — each in an isolated worktree and branch.

```bash
agent-desk add . -c claude --worktree feature/a --new-branch   # session in a new worktree
agent-desk worktree finish "My Session"                        # merge branch, remove worktree, delete session
agent-desk worktree cleanup                                    # remove orphaned worktrees
```

Set the default location in `~/.agent-desk/config.toml`:

```toml
[worktree]
default_location = "subdirectory"  # "sibling" (default), "subdirectory", or a custom path
```

### Multi-tool support

| Tool | Integration |
|------|-------------|
| **Claude Code** | Full — status, MCP, fork, resume, model detection |
| **Gemini CLI** | Full — status, MCP, resume |
| **GitHub Copilot CLI** | Status detection, resume (`--continue`), organization |
| **OpenCode / Codex / Cursor** (terminal) | Status detection + organization |
| **Any other terminal CLI** | Wrap it via `[tools.*]` in `config.toml` |

Any terminal-based AI tool can be added as a custom tool with its own launch command, icon, and status patterns:

```toml
[tools.my-ai]
command = "my-ai-assistant"
icon = "🧠"
busy_patterns = ["thinking...", "processing..."]
```

## Documentation

| Guide | Contents |
|-------|----------|
| [CLI Reference](skills/agent-desk/references/cli-reference.md) | Commands, flags, scripting |
| [Configuration](skills/agent-desk/references/config-reference.md) | `config.toml`, MCP, custom tools, socket pool |
| [TUI Reference](skills/agent-desk/references/tui-reference.md) | Shortcuts, status indicators, navigation |
| [Troubleshooting](skills/agent-desk/references/troubleshooting.md) | Common issues, recovery, uninstalling |

**Ask an AI about Agent Desk** — if you use Claude Code, install the bundled skill:

```bash
/plugin marketplace add millwright-software/agent-desk
/plugin install agent-desk@agent-desk
```

## FAQ

<details>
<summary><b>How is this different from just using tmux?</b></summary>

Agent Desk adds AI-specific intelligence on top of tmux: status detection that knows when Claude is working vs.
waiting, session forking with context inheritance, MCP management, global search across conversations, and
organized groups. It's tmux plus AI awareness.

</details>

<details>
<summary><b>Can I use it on Windows?</b></summary>

Yes, via WSL. [Install WSL](https://learn.microsoft.com/en-us/windows/wsl/install) (WSL2 recommended), then
install Agent Desk inside it.

</details>

<details>
<summary><b>Will it interfere with my existing tmux setup?</b></summary>

No. Agent Desk creates its own tmux sessions prefixed `agentdesk_*`; your existing sessions are untouched.

</details>

## Development

```bash
make build    # Build
make test     # Test
make lint     # Lint
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE). Original work © 2025 Ashesh Goplani; fork modifications © 2026 Millwright Software.

---

<div align="center">

Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [tmux](https://github.com/tmux/tmux).

**[Docs](skills/agent-desk/references/) · [Issues](https://github.com/millwright-software/agent-desk/issues) · [Discussions](https://github.com/millwright-software/agent-desk/discussions)**

</div>
