# CLI Command Reference

Complete reference for all agent-desk CLI commands.

## Table of Contents

- [Global Options](#global-options)
- [Basic Commands](#basic-commands)
- [Session Commands](#session-commands)
- [MCP Commands](#mcp-commands)
- [Group Commands](#group-commands)
- [Profile Commands](#profile-commands)

## Global Options

```bash
-p, --profile <name>    Use specific profile
--json                  JSON output
-q, --quiet             Minimal output
```

## Basic Commands

### add - Create session

```bash
agent-desk add [path] [options]
```

| Flag | Description |
|------|-------------|
| `-t, --title` | Session title |
| `-g, --group` | Group path |
| `-c, --cmd` | Command (claude, gemini, opencode, codex, custom) |
| `--parent` | Parent session (creates child) |
| `--mcp` | Attach MCP (repeatable) |

```bash
agent-desk add -t "My Project" -c claude .
agent-desk add -t "Child" --parent "Parent" -c claude /tmp/x
agent-desk add -t "Research" -c claude --mcp exa --mcp firecrawl /tmp/r
```

### list - List sessions

```bash
agent-desk list [--json] [--all]
agent-desk ls  # Alias
```

### remove - Remove session

```bash
agent-desk remove <id|title>
agent-desk rm  # Alias
```

### status - Status summary

```bash
agent-desk status [-v|-q|--json]
```

- Default: `2 waiting - 5 running - 3 idle`
- `-v`: Detailed list by status
- `-q`: Just waiting count (for scripts)

## Session Commands

### session start

```bash
agent-desk session start <id|title> [-m "message"] [--json] [-q]
```

`-m` sends initial message after agent is ready.

**CRITICAL:** Flags MUST come BEFORE session name!
```bash
# Correct
agent-desk session start -m "Hello" my-project

# WRONG - flag ignored!
agent-desk session start my-project -m "Hello"
```

### session stop

```bash
agent-desk session stop <id|title>
```

### session restart

```bash
agent-desk session restart <id|title>
```

Reloads MCPs without losing conversation (Claude/Gemini).

### session attach

```bash
agent-desk session attach <id|title>
```

Interactive PTY mode. Press `Ctrl+Q` to detach.

### session show

```bash
agent-desk session show [id|title] [--json] [-q]
```

Auto-detects current session if no ID provided.

**JSON output includes:**
- Session details (id, title, status, path, group, tool)
- Claude/Gemini session ID
- Attached MCPs (local, global, project)
- tmux session name

### session current

```bash
agent-desk session current [--json] [-q]
```

Auto-detect current session and profile from tmux environment.

```bash
# Human-readable
agent-desk session current
# Session: test, Profile: work, ID: c5bfd4b4, Status: running

# For scripts
agent-desk session current -q
# test

# JSON
agent-desk session current --json
# {"session":"test","profile":"work","id":"c5bfd4b4",...}
```

**Profile auto-detection priority:**
1. `AGENTDESK_PROFILE` env var
2. Parse from `CLAUDE_CONFIG_DIR` (`~/.claude-work` -> `work`)
3. Config default or `default`

### session set

```bash
agent-desk session set <id|title> <field> <value>
```

**Fields:** title, path, command, tool, claude-session-id, gemini-session-id

### session send

```bash
agent-desk session send <id|title> "message" [--no-wait] [-q] [--json]
```

Default: Waits for agent readiness before sending.

### session output

```bash
agent-desk session output [id|title] [--json] [-q]
```

Get last response from Claude/Gemini session.

### session set-parent / unset-parent

```bash
agent-desk session set-parent <session> <parent>
agent-desk session unset-parent <session>
```

## MCP Commands

### mcp list

```bash
agent-desk mcp list [--json] [-q]
```

### mcp attached

```bash
agent-desk mcp attached [id|title] [--json] [-q]
```

Shows MCPs from LOCAL, GLOBAL, PROJECT scopes.

### mcp attach

```bash
agent-desk mcp attach <session> <mcp> [--global] [--restart]
```

- `--global`: Write to Claude config (all projects)
- `--restart`: Restart session immediately

### mcp detach

```bash
agent-desk mcp detach <session> <mcp> [--global] [--restart]
```

## Group Commands

### group list

```bash
agent-desk group list [--json] [-q]
```

### group create

```bash
agent-desk group create <name> [--parent <group>]
```

### group delete

```bash
agent-desk group delete <name> [--force]
```

`--force`: Move sessions to parent and delete.

### group move

```bash
agent-desk group move <session> <group>
```

Use `""` or `root` to move to default group.

## Profile Commands

```bash
agent-desk profile list
agent-desk profile create <name>
agent-desk profile delete <name>
agent-desk profile default [name]
```

## Session Resolution

Commands accept:
- **Title:** `"My Project"` (exact match)
- **ID prefix:** `abc123` (6+ chars)
- **Path:** `/path/to/project`
- **Current:** Omit ID in tmux (uses env var)

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error |
| 2 | Not found |
