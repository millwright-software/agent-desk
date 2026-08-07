# Cross-Session Messaging for Claude Code

**Status:** Idea sketch — discuss with Anthropic
**Date:** 2026-02-26

## Problem

With multiple Claude Code sessions running simultaneously (e.g., 12 sessions across different projects), there's no way for one session to send context or notifications to another. Each session is fully isolated.

## Use Cases

- Session A discovers something Session B needs to know (e.g., "I changed the API interface you're consuming")
- Coordinating parallel work — "I'm done with the auth refactor, you can proceed"
- Broadcasting warnings — "Don't touch migrations, I'm running a schema change"

## Sketch: File-Based Message Queue + Hook

### Architecture

```
~/.claude/messages/
├── inbox/
│   ├── <session-id-1>.jsonl    # Messages waiting for session 1
│   └── <session-id-2>.jsonl    # Messages waiting for session 2
├── registry.json               # Active sessions: {id, project, branch, pid, started}
└── archive/                    # Processed messages
```

### Message Format

```jsonl
{"from": "session-abc", "project": "agent-desk", "timestamp": "2026-02-26T10:00:00Z", "type": "info", "body": "Changed UserConfig struct — added SidebarWidth field"}
```

### Components

1. **Registry** — Sessions register on start, deregister on shutdown. Maps session IDs to project/branch for addressing.

2. **Send** — CLI command or skill: `/notify <target> <message>`
   - Target by session ID, project name, or broadcast
   - Writes a line to target's inbox file

3. **Receive hook** — `PreToolUse` or `user-prompt-submit` hook
   - Checks `~/.claude/messages/inbox/<my-session-id>.jsonl`
   - If messages exist, reads them, injects as context, moves to archive
   - Lightweight: just a file existence check on each turn

4. **List** — `/sessions` to see active sessions and their projects

### Hook Implementation (receive side)

```bash
#!/bin/bash
# ~/.claude/hooks/check-inbox.sh
INBOX="$HOME/.claude/messages/inbox/${CLAUDE_SESSION_ID}.jsonl"
if [ -f "$INBOX" ] && [ -s "$INBOX" ]; then
    echo "--- MESSAGES FROM OTHER SESSIONS ---"
    cat "$INBOX"
    echo "--- END MESSAGES ---"
    mv "$INBOX" "$HOME/.claude/messages/archive/$(date +%s)-${CLAUDE_SESSION_ID}.jsonl"
fi
```

### Send Implementation

```bash
#!/bin/bash
# /notify skill or CLI wrapper
TARGET_SESSION="$1"
shift
MESSAGE="$*"
SENDER="${CLAUDE_SESSION_ID:-unknown}"

echo "{\"from\":\"$SENDER\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"body\":\"$MESSAGE\"}" \
  >> "$HOME/.claude/messages/inbox/${TARGET_SESSION}.jsonl"
```

## Open Questions for Anthropic

1. **Is there a session ID env var?** Need `CLAUDE_SESSION_ID` or equivalent exposed to hooks
2. **Hook context injection** — Can hook output be injected into the LLM context, or just shown to the user? Need it in context for the session to act on it.
3. **Any plans for native IPC?** Maybe this is already on the roadmap
4. **Rate/size limits on hook output?** If a session gets 50 messages, injecting all at once could blow context
5. **Could MCP servers help here?** An MCP server running as a singleton could act as the message broker instead of files

## Relationship to Handoff Framework

The existing handoff framework does coarse-grained state transfer (shutdown → start). This would be fine-grained, real-time messaging between live sessions. They complement each other:
- Handoff = async state persistence across session lifecycles
- Messaging = sync communication between concurrent sessions
