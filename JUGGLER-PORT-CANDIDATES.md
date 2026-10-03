# Juggler port candidates (juggler-ai/juggler → agent-desk)

Survey run 2026-10-02 against juggler `main` (last commit 2026-10-02). Juggler is
Julian Storer's "visual workbench for AI coding agents": a Go server plus a web
and desktop UI, ~225k lines. It runs its own LLM loop and owns the conversation
document. agent-desk owns neither; it manages tmux sessions running someone
else's agent and reads their transcripts. That difference decides most of what
follows.

## Licence first: ideas yes, code no

| Part of juggler | Licence | Usable by agent-desk (MIT)? |
|---|---|---|
| Go server, desktop app, web UI, docs, tests | AGPL-3.0-or-later | **No.** Copying any of it in would put agent-desk under AGPL. |
| `web/sdk/**`, `web/extensions/**`, `examples/**` | Apache-2.0 | Licence-compatible, but it is JavaScript for juggler's extension host. Nothing a Go TUI can use. |

So this is a list of designs to re-implement, not code to lift. Every item below
was read in juggler's source and checked against what agent-desk has today.

## What "the tree" actually is

Juggler's tree is the **conversation**, not the session list. One conversation
is a document whose items are typed (`user`, `assistant`, `thinking`,
`tool-action`, `thread`, `system-prompt`, ...). A `thread` item owns its own
list of items, so nesting is by containment; a sub-thread runs in isolation and,
when it rests, its result is handed back to the parent as a tool result. The UI
is Miller columns (`web/js/utils/column-selection.js`): column 0 is the root
conversation; selecting a thread opens its items as the next column; selecting
a folded group of tool calls ("4 tools: 2× Replace Text, 1× Read File, 1×
Search") opens the calls as the next column; selecting anything else opens an
inspector column. Right arrow drills in, left backs out, Finder-style
truncation. On top of that sit structural edits (fold history into a thread,
move or copy items, expand a thread back into its parent) with undo.

**The structural edits cannot be ported.** They require being the harness that
assembles the next prompt. Claude Code is that harness, not agent-desk.

**The navigation can.** Claude Code's transcript JSONL already has everything
needed to rebuild the same tree read-only: every record carries `uuid`,
`parentUuid`, `isSidechain`, `type`; assistant messages contain `tool_use`
blocks and user messages contain the matching `tool_result`; sub-agents write
their own `agent-*.jsonl` next to the parent. Checked against a live transcript
in `~/.claude/projects/` during this survey.

## Tier 1 — small, clear value, do first

- **Next-session-that-needs-you key.** Juggler's ⌘J (`services/conversation-commands.js:134`)
  jumps to the next session awaiting approval, falls back to the next running
  one, and lands with the blocking item on screen. agent-desk already has the
  ring walk (`neighbourInRing` in `internal/ui/home.go`) and a `waiting` status
  from Claude hooks. A second ring with the predicate `status == waiting` is an
  evening's work: one key while attached (and one in the sidebar) that cycles
  only sessions blocked on you. The landing card already says where you are.
- **Waiting labels that restate as time grows.** `services/status-message-builder.js`:
  "a wait of a few seconds and a wait of ten minutes are different situations",
  so the label changes wording, not just the number. agent-desk shows a status
  dot and nothing about how long it has been waiting. Cheap, and it is the
  thing that tells you which green dot to go to first.
- **Colour by hash for worktree sessions.** `utils/workspace-colour.js` derives a
  hue from the workspace id (FNV-1a into a small set of theme tokens): no stored
  field, same colour everywhere, "a hue is read before a word is" when every
  branch name shares its first twenty characters. agent-desk sessions in
  worktrees could tint the branch badge the same way.

## Tier 2 — the tree, scoped to what agent-desk can own

- **Read-only transcript tree in the preview pane.** A third preview mode
  (the `v` key already cycles three modes, but the analytics one is hardcoded
  off in `renderPreviewPane`, so the slot is free). Column 1: the session's
  turns, with adjacent tool calls folded into one row the way juggler does
  (`utils/item-grouping.js`). Right arrow on a folded row: the individual calls.
  Right arrow on a call: its input and result. Sub-agent transcripts
  (`agent-*.jsonl`, which agent-desk currently skips in
  `findActiveSessionIDExcluding`) appear as drillable thread rows, which is the
  part juggler's screenshots make look good: a delegated thread as one row with
  a live timer.

  Why it is worth it here specifically: with ten sessions open you can see what
  a parked or background session did without attaching to it and scrolling
  tmux history. That is a different job from Claude Code's own inline display.

  What it costs, from the survey of agent-desk:
  - A real transcript parser. Today there are three unrelated minimal structs
    (`jsonlEntry` in `analytics.go`, `claudeJSONLRecord` in `global_search.go`,
    `claudeRecord` in `instance.go`) and none reads `uuid`, `parentUuid`,
    `isSidechain` or `tool_result`. One parser that builds the tree, with the
    EOF-anchored reading that `UPSTREAM-PORT-CANDIDATES.md` already flags for
    `/compact`-sized records.
  - A scrolling viewport. There is none: the preview pane prints the last N
    lines and `bubbles/viewport` is not imported anywhere. `internal/ui/preview.go`
    and `internal/ui/tree.go` are dead code and not a starting point.
  - The column renderer and keys. Juggler's keyboard model
    (`conversation-tab.js:_setupKeyboardNavigation`) is the one to copy: ↑/↓ in
    the column, → drills only if the row is drillable and selects its first
    child, ← backs out, no separate expand/collapse.

  MVP cut: two columns (turns, then the selected turn's tool calls) and a detail
  pane for one call. Sub-agent rows second. Live follow of a running session
  third.

- **Awaiting-approval outranks running.** Juggler derives one per-session truth
  (`conversation-bar.js:_conversationActivity`): if anything in the tree is
  awaiting a human, the session is "awaiting", even though the worker still
  reports "processing". agent-desk's hook mapping already produces `waiting`
  from `PermissionRequest`, so the state exists; the port is the rule that it
  wins over pattern-detected `running`, and that a destructive op (delete) is
  gated on running, never on waiting, because a waiting session executes
  nothing.

- **Alerts on edges, gated on attention, cleared on view.**
  `utils/attention-manager.js` (its 70-line header is the spec): fire only on a
  false→true edge, only if the user is not already looking at that session,
  and keep the standing mark until the session is viewed. agent-desk's bell
  (v1.0.4) fires on the transition already; what is missing is the "not if you
  are attached to it" gate and the standing mark that clears on attach, which
  the `u`-key unread flag could become automatically.

## Tier 3 — real but lower priority

- **Git status badge per session.** `services/git-status-cache.js`: one shared
  poll, only while something is watching and the window is focused, with
  `git status --no-optional-locks`, reading the session's worktree rather than
  the project. Branch and dirty count in the sidebar row. Separate from an
  on-demand full diff, which juggler never caches.
- **Workspace reconcile at startup.** `services/workspace-reconcile.js`: squares
  the persisted worktree table against disk after a restart, under a
  server-side claim so two clients do not `git worktree prune` at each other.
  agent-desk has a manual `worktree cleanup` and already elects a primary
  process in `statedb`; running cleanup once from the primary at startup is
  the port.
- **Checkpointed worktree lifecycle.** From `docs/extension_guide.md` (~880 to
  1020): every irreversible step records its compensation before taking it,
  compensations tolerate absence, and the finish dialog describes what happens
  to the sessions inside, not just the files. agent-desk's `worktree finish`
  merges, removes, deletes in sequence; a failure mid-way leaves no record of
  what to undo.
- **Context meter honesty rules** (`components/token-display.js`,
  `docs/context-window.md`) if the analytics panel comes back: the count is
  always neutral grey, the bar only renders when the limit is known, windows
  that are guessed are labelled "assumed", fill goes grey/amber/red with no
  green. agent-desk's `renderContextBar` assumes 200k silently.

## Not worth porting

- The CRDT document, multi-client sync, and per-column composers. They exist
  because juggler is the harness. agent-desk's sync layer is tmux.
- Provider layer, extensions, MCP postman, pinboard, scheduled send,
  disconnection overlay, custom slash commands. Different product.
- Chime synth: agent-desk has the bell. Bin with undo: agent-desk has Ctrl+Z
  undo delete.
- Terminal single-key verbs and QR code on the headless server: agent-desk is
  already a TUI.

## Suggested order

1. Next-waiting key and the elapsed-wait label (Tier 1, one sitting each).
2. Transcript parser that builds the tree, used first for a one-line "last
   tool calls" summary per session. This is the foundation for the tree view
   and is useful on its own.
3. The tree preview mode on top of it, two columns first.
4. Awaiting-outranks-running and the attention gate.
5. Git badge and startup reconcile.
