# Changelog

All notable changes to this fork of Agent Desk are documented here.

This is a [Millwright Software](https://github.com/millwright-software) fork of
[`asheshgoplani/agent-deck`](https://github.com/asheshgoplani/agent-deck). The pre-fork release history lives
upstream; this file tracks changes made in the fork. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); no versioned release has been cut yet.

## [Unreleased]

## [1.0.7] - 2026-08-25

### Removed
- **Deleted the session-forking feature entirely** (~2,200 lines). It had been dormant/unreachable since the
  keybindings were unwired, and v1.0.6 stopped advertising it; this removes the code: the fork dialog, all
  `home.go` fork handlers and animation state, the session-package primitives (`CanFork`, `Fork*`,
  `CreateForkedInstance*`, `ForkOpenCode*`, `writeOpenCodeForkScript`), the `ToArgsForFork` option methods and
  their fork-only transient fields, the `session fork` CLI subcommand, and all fork tests. No behavior change —
  fork was already unusable.

## [1.0.6] - 2026-08-24

### Removed
- **Fork is no longer advertised.** The fork keybindings were already unwired in this fork (dead code), but the
  help screen and README still listed `f`/`F`, a `session fork` CLI example, and a "Fork sessions" section.
  Removed those so the docs match reality. The dormant fork code remains in the tree.

### Fixed
- **Corrected the README status table.** It claimed Running=green / Waiting=yellow with a `◐` symbol; the actual
  UI is Running=**yellow** `●`, Waiting=**green** `●`. Also documented the dimmer-green unread bookmark (`u`) vs
  the bright-green waiting state.

## [1.0.5] - 2026-08-22

### Fixed
- **Worktrees no longer nest inside worktrees.** Creating a new session (or forking) while the base directory
  was itself a git worktree produced `.worktrees/` inside a worktree, because the base resolved to the
  worktree's own root. New worktrees now always base off the **main repo root** (de-nested via
  `GetWorktreeBaseRoot`).

### Changed
- **New Session worktree UX is a single name.** Removed the separate branch field: the session name is the one
  input, and it fills both the branch and the worktree folder — identical, sanitized, and **lowercased**
  (e.g. "Auth Refactor" → folder `auth-refactor`, branch `auth-refactor`, no prefix).
- **The dialog now shows the full resolved worktree path** (de-nested, `~`-collapsed) and the derived branch,
  live as you type — so you can see exactly where the worktree will land before creating it. Says so when the
  selected path isn't a git repo.

## [1.0.4] - 2026-08-22

### Added
- **Attention bell.** Rings the terminal bell once when any session transitions into "waiting" (needs your
  input) — so you hear it even while attached to another session or looking away. One ring per poll regardless
  of how many sessions transition. On by default; disable with `bell = false` under `[notifications]` in
  `~/.agent-desk/config.toml`.

### Changed
- **New sessions are inserted directly below the highlighted row** instead of at the bottom of the group. The
  position persists (renormalized `Order`), and falls back to appending when there's no same-group anchor.
  Forking is unchanged (forks already nest under their parent).
- **Distinct greens for the two "green" states.** "Needs attention / not yet looked at" (waiting) stays bright
  green; the `u` unread bookmark ("seen it, reply still owed") is now a dimmer green — they were previously the
  same color and indistinguishable.

## [1.0.3] - 2026-08-07

### Added
- **Light color schemes** (`Paper`, `Daylight`, `Linen`) selectable with `c`, alongside the existing dark presets.

### Fixed
- **Copilot text unreadable (dark-on-dark).** Copilot CLI mis-detects a dark terminal — its background-detection
  query is swallowed inside tmux — so it renders as if on a light terminal and prints dark text. Rather than
  fight its detection, new Copilot sessions now default to a **light pane** (`Paper`), so Copilot's dark text is
  readable. Change it per session with `c`; for an existing Copilot session, press `c` and pick a light scheme.

## [1.0.0] - 2026-08-06

First tagged release of the Millwright Software fork.

### Added
- **GitHub Copilot CLI** as a first-class tool: selectable in the new-session dialog and setup wizard, with its
  own icon, name detection, `copilot --continue` resume on restart, and status-detection patterns adapted from
  upstream agent-deck's real Copilot CLI transcripts.
- Manual session flags: press `u` to cycle a session through unread → parked (red) → parked (blue) → normal;
  the flag clears automatically when you attach.
- Model and mode (auto/manual) surfaced in the navigation rows, with shorthand model names (e.g. `o4.8`, `g2.5p`).
- Name-forward new-session path picker: rows lead with the folder name, collapse `$HOME` to `~`, and mark recent
  projects with a star.

### Changed
- Session saves are now upsert-only, so multiple concurrent Agent Desk windows no longer delete each other's
  sessions (previous full-table-sweep saves were a data-loss vector).
- "Last active" time is derived from session-log activity and shown as minute-rounded shorthand.
- The attached tmux status bar now shows the session's own title.
- Claude model detection refreshes after a `/model` switch, even for idle sessions.
- The numbered quick-switch / notification bar defaults off.

### Removed
- The **conductor** meta-agent orchestration subsystem (persistent orchestrator sessions + Telegram/Slack
  bridges) — an agent-deck feature this fork doesn't use (~5,000 lines).
- The per-session context-note feature.
- Stale upstream marketing site, SEO files, and the `llms-full.txt` dump.

### Known limitations
- Orphaned `tmux -C` control clients left by crashed/killed TUIs are not yet swept, so on very heavy long-term
  use they could accumulate toward the OS pty cap. Upstream's fix is a ~300-line platform-specific port and is
  deferred; clean exits don't orphan, and the primary "can't create sessions" cause (concurrent-window save
  sweeps) is already fixed above.
