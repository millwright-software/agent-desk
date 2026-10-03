# 2026-10-03 — Restart after a tmux-server kill resumed sibling instances' sessions

## What happened

Eric killed the tmux server (macOS permission cache had cut the long-running
server off from the desktop), relaunched iTerm and Agent Desk, and resumed three
instances. Each one came up inside a different instance's conversation:

| Instance  | Stored before | Resumed as   | Which belonged to |
|-----------|---------------|--------------|-------------------|
| William   | a632259e…     | d88238d1…    | dnd               |
| workbench | 43aab9f8…     | 63638321…    | parent-rep        |
| ios home  | d01369c0…     | 975c3a5f…    | 🏡 Home           |

## Why

`Restart()` calls `syncClaudeSessionFromDisk()` before resuming so a `/clear`
inside a live pane is picked up. That scan takes the most recently modified
`.jsonl` in the project directory, excluding only IDs held in the `CLAUDE_SESSION_ID`
env of other *live* agent-desk tmux sessions. After the server kill there were no
live sessions to exclude, every vault instance shares one project path, and the
quality gate accepts real→real ("newer wins"). So each restart adopted whichever
sibling's transcript had been touched last. The SessionStart hook then wrote the
wrong ID into the instance's hook file, and `loadExisting()` would have re-applied
it on every later launch.

## Fix

- `Instance.shouldSyncFromDiskOnRestart()`: the rescan only runs while the tmux
  session exists. With the pane gone, a stored ID whose file has conversation data
  is authoritative. Empty or zombie stored IDs still rescan.
- Test: `TestShouldSyncFromDiskOnRestart_DeadPaneKeepsStoredID`.
- Repair done by hand (binary rebuilt with `make install`): the three rows in
  `~/.agent-desk/profiles/default/state.db` and the three hook files in
  `~/.agent-desk/hooks/` set back to the stored-before IDs above. All sixteen
  instances verified to point at an existing transcript. Nine orphaned
  `tmux -C attach-session` clients from an August run (ppid 1, no server) killed.

## Open

- The exclusion set could also include IDs stored on other instances in the DB,
  not just live tmux envs. Not done; the dead-pane guard closes the hole that bit.
- Not committed in this pass. `git diff` shows the two files.
