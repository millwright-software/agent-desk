# Troubleshooting Guide

Common issues and solutions for agent-desk.

## Quick Fixes

| Issue | Solution |
|-------|----------|
| Session shows `✕` error | `agent-desk session start <name>` |
| MCPs not loading | `agent-desk session restart <name>` |
| CLI changes not in TUI | Press `Ctrl+R` to refresh |
| Flag not working | Put flags BEFORE arguments |
| Status stuck | Wait 2 seconds or press `u` to mark unread |

## Common Issues

### Flags Ignored

**Problem:** Flags after positional arguments are silently ignored.

```bash
# WRONG - message not sent
agent-desk session start my-project -m "Hello"

# CORRECT
agent-desk session start -m "Hello" my-project
```

### MCP Not Available

1. Check if attached: `agent-desk mcp attached <session>`
2. Restart session: `agent-desk session restart <session>`
3. Verify in config: `agent-desk mcp list`

### Session ID Not Detected

Claude session ID is needed to resume a session (Shift+R). Check:

```bash
agent-desk session show <name> --json | jq '.claude_session_id'
```

If null, restart session and interact with Claude.

### High CPU Usage

**With many sessions:** Normal if batched updates. Check:
```bash
agent-desk status  # Should show ~0.5% CPU when idle
```

**With active session:** Normal (live preview updates).

### Log Files Too Large

Add to `~/.agent-desk/config.toml`:
```toml
[logs]
max_size_mb = 1
max_lines = 2000
```

### Global Search Not Working

Check config:
```toml
[global_search]
enabled = true
```

Also verify `~/.claude/projects/` exists and has content.

## Debugging

Enable debug logging:
```bash
AGENTDESK_DEBUG=1 agent-desk
```

Check session logs:
```bash
tail -100 ~/.agent-desk/logs/agentdesk_<session>_*.log
```

## Report a Bug

If something isn't working, please create a GitHub issue with all relevant context.

### Step 1: Gather Information

Run these commands and save output:

```bash
# Version info
agent-desk version

# Current status
agent-desk status --json

# Session details (if session-related)
agent-desk session show <session-name> --json

# Config (sanitized - removes secrets)
cat ~/.agent-desk/config.toml | grep -v "KEY\|TOKEN\|SECRET\|PASSWORD"

# Recent logs (if error occurred)
tail -100 ~/.agent-desk/logs/agentdesk_<session>_*.log 2>/dev/null

# System info
uname -a
echo "tmux: $(tmux -V 2>/dev/null || echo 'not installed')"
```

### Step 2: Describe the Issue

Prepare clear answers to:

1. **What did you try?** (exact command or TUI action)
2. **What happened?** (error message, unexpected behavior)
3. **What did you expect?** (correct behavior)
4. **Can you reproduce it?** (steps to trigger)

### Step 3: Create GitHub Issue

Go to: **https://github.com/millwright-software/agent-desk/issues/new**

Use this template:

```markdown
## Description

[Brief description of the issue]

## Steps to Reproduce

1. [First step]
2. [Second step]
3. [What happened]

## Expected Behavior

[What should have happened]

## Environment

- agent-desk version: [output of `agent-desk version`]
- OS: [macOS/Linux/WSL]
- tmux version: [output of `tmux -V`]

## Debug Output

<details>
<summary>Status JSON</summary>

```json
[paste agent-desk status --json]
```

</details>

<details>
<summary>Config (sanitized)</summary>

```toml
[paste sanitized config]
```

</details>

<details>
<summary>Logs</summary>

```
[paste relevant log lines]
```

</details>
```

### Step 4: Follow Up

- Check for responses on your issue
- Test any suggested fixes
- Update issue with results
- Open a [GitHub issue](https://github.com/millwright-software/agent-desk/issues) or start a [discussion](https://github.com/millwright-software/agent-desk/discussions)

## Recovery

### Session Metadata Lost

Data stored in SQLite:
```bash
~/.agent-desk/profiles/default/state.db
```

Recovery (if state.db is corrupted):
```bash
# If sessions.json.migrated still exists, delete state.db and restart.
# agent-desk will auto-migrate from the .migrated file.
rm ~/.agent-desk/profiles/default/state.db
mv ~/.agent-desk/profiles/default/sessions.json.migrated \
   ~/.agent-desk/profiles/default/sessions.json
# Restart agent-desk to trigger auto-migration into a fresh state.db
```

### tmux Sessions Lost

Session logs preserved:
```bash
tail -500 ~/.agent-desk/logs/agentdesk_<session>_*.log
```

### Profile Corrupted

Create fresh:
```bash
agent-desk profile create fresh
agent-desk profile default fresh
```

## Uninstalling

Remove agent-desk from your system:

```bash
agent-desk uninstall              # Interactive uninstall
agent-desk uninstall --dry-run    # Preview what would be removed
agent-desk uninstall --keep-data  # Remove binary only, keep sessions
```

Or use the standalone script:
```bash
curl -fsSL https://raw.githubusercontent.com/millwright-software/agent-desk/main/uninstall.sh | bash
```

**What gets removed:**
- **Binary:** `~/.local/bin/agent-desk` or `/usr/local/bin/agent-desk`
- **Homebrew:** `agent-desk` package (if installed via brew)
- **tmux config:** The `# agent-desk configuration` block in `~/.tmux.conf`
- **Data directory:** `~/.agent-desk/` (sessions, logs, config)

Use `--keep-data` to preserve your sessions and configuration.

## Critical Warnings

**NEVER run these commands - they destroy ALL agent-desk sessions:**

```bash
# DO NOT RUN
tmux kill-server
tmux ls | grep agentdesk | xargs tmux kill-session
```

**Recovery impossible** - metadata backups exist but tmux sessions are gone.
