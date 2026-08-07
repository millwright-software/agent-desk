# Changelog

All notable changes to this fork of Agent Desk are documented here.

This is a [Millwright Software](https://github.com/millwright-software) fork of
[`asheshgoplani/agent-deck`](https://github.com/asheshgoplani/agent-deck). The pre-fork release history lives
upstream; this file tracks changes made in the fork. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); no versioned release has been cut yet.

## [Unreleased]

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
