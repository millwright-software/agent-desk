# 2026-11-11 — The file-limit banner says restart, and what it costs (PR #5, v1.0.29)

**Ask:** "Get up to speed on my agent desk software. There's a PR from my work
account to make some changes." Then, after the review: "fix 1 and 2, merge, cut
v1.0.29".

**The PR (er0826-dex, #5):** on a tmux server stuck at the old 256 default the
banner said "Close an extra agent-desk window". For that server the advice is
wrong twice: the cap stays whatever closes, and one window with about 19
sessions fills it alone, so there is no extra window. The PR added
`ServerFiles.Relief()`: a `Low()` server gets "restart the tmux server", a
roomier one keeps "Close an extra agent-desk window", and all three branches
of `tmuxServerWarning()` use it. Reproduced on a live 256-capped server (197
files, 19 sessions).

## Review findings, both fixed before the merge

1. **The banner pointed at the destructive fix without its cost.** A tmux
   server restart ends every session in it. The troubleshooting reference
   lists it third of "safest first" and says only the user can choose. The
   "server is full" branch would have read "agents still running. restart the
   tmux server". `Relief()` for a low server now reads: "Only a tmux server
   restart lifts its limit, and that ends every session (R resumes Claude
   ones). Troubleshooting: tmux file limit".
2. **The "server is full" branch could not see the limit.** `Relief()` read
   the cached probe; `ProbeServerFiles` needs a tmux client; a full server
   refuses clients. Within one `roomTTL` (15 s) the failed probe overwrote
   `pm.room` with a zero reading, `Low()` went false, and the banner fell back
   to "Close an extra window" on exactly the server the PR was written for.
   `PipeManager` now keeps `lastRoom`, the last reading a probe returned
   without error, behind `LastRoom()`, and that branch uses it. `pm.room` is
   untouched: `takeRoom`, `roomTTL` and the other two banner branches read it
   as before (filling `room.Limit` on error instead would have stretched the
   TTL to five minutes on a roomy server and could have fired the low-limit
   notice off a stale count).

Also: the low-limit notice had lost its reason; "another agent-desk window
won't fit" is back ahead of the instruction.

## Known gaps

- An agent-desk that starts against an already-full server has no good
  reading, so its full-server banner says "Close an extra agent-desk window".
- `tmux kill-server` cannot reach a full server either. The restart there
  means closing a window first or killing the server's pid. The banner does
  not say this; the troubleshooting reference does not either.
- The banner strings in `tmuxServerWarning()` have no test of their own;
  `TestRelief` and `TestLastRoomSurvivesFailedProbe` cover the pieces.

## Release

- PR #5 merged by rebase: `ef8e18e8` (the PR), `76800926` (the two fixes,
  pushed to the PR branch on the fork), then `472222d7` Release v1.0.29.
- `go test -race ./...` green across all 13 packages; `gofmt -l` empty.
- Cut with `goreleaser release --clean`, `GITHUB_TOKEN` and
  `HOMEBREW_TAP_GITHUB_TOKEN` both from `gh auth token`. Four archives and
  `checksums.txt` published; v1.0.29 is Latest.
- **The Homebrew cask step failed: `millwright-software/homebrew-tap` does not
  exist (404).** It runs after the GitHub release publishes, so the binary
  install and `agent-desk update` are unaffected, but goreleaser exits
  non-zero and the release notes advertise `brew install
  millwright-software/tap/agent-desk`, which does not work. Open: create the
  tap, or take `homebrew_casks` and the brew line out of `.goreleaser.yml`.
- `skills/agent-desk/SKILL.md` `**Version:**` was at 1.0.20 through eight
  releases (the same drift the 9/6 checklist note warned about); now 1.0.29.
  The release step still has nothing that enforces it.
- Not installed locally; the running instance was not restarted.
