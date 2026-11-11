# 2026-11-11 — The tmux server's file limit (#3, #4)

**Ask:** "Check GH for issues", then "Can you do both?" and "find that second
bug in the other one" (the leftover clients in #4).

**Finding:** one cause under both issues. The tmux server ran out of open
files. Neither the socket location nor control-mode fan-out alone explains
it; fan-out against a 256-file cap does.

## Evidence

All experiments ran on isolated servers (their own `TMUX_TMPDIR`, `TMUX`
unset), with agent-desk state in a throwaway `HOME`. The live server was read
only (`run-shell 'ulimit -n'`, `lsof -p`, `ps`), never written to.

- **The live server's limit is 256.** `tmux run-shell 'ulimit -n'` → 256
  (hard: unlimited). With one agent-desk window and 25 sessions it held 117
  files: 25 control clients plus the panes'.
- **A control client costs the server three files**: its socket plus the
  stdin and stdout tmux passes over it. Measured: 5 clients raised the count
  from 34 to 49.
- **At the limit the server is unreachable but alive.** Capped at 64, the
  20th client tipped it: `tmux list-sessions` → `server exited unexpectedly`
  (exit 1, no hang) while 18 clients and every pane stayed up. That is #4's
  incident exactly.
- **Real windows reproduce #3.** Six sessions, server capped at 64: window 1
  took the server to 38 files, window 2 to 56, and window 3 tipped it after
  connecting 5 of its 6 pipes. Then every window, including the first:
  previews stuck on "Loading preview...", attach did nothing, the session
  without a pipe went ✕, and the status loop took 4.9 s. Keys still moved
  the cursor. To a user that is a frozen desk.
- **Recovery needs no kill.** Quitting window 3 freed its clients; the server
  answered at once and window 1's preview came back.
- **Why agent-desk's servers had 256.** Go raises its own soft limit at
  startup (to 122880 here) but restores the original in every child it
  execs, unless the program calls `Setrlimit` itself. Verified with a small
  program: child gets 256 without the call, 10240 with it. So `tmux
  new-session` from agent-desk started a server capped at 256.
- **Not the socket.** macOS's `/usr/libexec/tmp_cleaner` (daily at midnight)
  deletes only regular files (`-type f`) and empty directories under `/tmp`;
  a socket is neither, so it never removes `/tmp/tmux-<uid>/default`. A
  reboot clears `/tmp`, but it ends the server too. A tmux upgrade under a
  running server can also produce this error; tmux here has been 3.6a since
  February, and the work laptop's history wasn't checked.
- **The 75 leftover clients need not be leaks.** Three or four windows on 19
  sessions is 57–76 clients. A window killed outright takes its clients with
  it: they exit when the parent dies (tested: 3 → 0 within 1.5 s).

## The second bug in #4: what can leak

Read from `internal/tmux/pipemanager.go` and `controlpipe.go`:

1. `watchPipe` dropped the pipe from the map on give-up or session-gone
   without `Close()`. If the reader had died while tmux stayed attached (a
   scanner error, e.g. an output line over 2 MB), that client lived on,
   holding three server files, read by no one.
2. `watchPipe` set `reconnecting[session]`, then called `Connect`, which
   returns early when that flag is set. Every reconnect was a silent no-op
   that logged `pipe_reconnected`; only the 20 s sweep restored pipes.
   `TestWatchPipeReconnects` fails on the old logic and passes on the new.
3. A client that died during the handshake (what a full server does to
   one) was returned as an error without `Wait`: a zombie per attempt, and
   the sweep retries every unpiped session every 20 s.

## Fix

- `tmux.RaiseFileLimit()`, first thing in `main`: soft limit 16384 (bounded
  by hard). A server agent-desk starts inherits it. Verified: from a shell
  at 256, the new build's server has 16384; v1.0.27's has 256.
- Pipe budget in `PipeManager.Connect`: probe the server (`display -p
  '#{pid}'`, `run-shell 'ulimit -n'`, `lsof -p`; about 70 ms), cache it,
  debit three files per pipe, keep 64 back for transient commands. With no
  room, Connect refuses (`ErrNoServerRoom`) and the session is polled by
  subprocess instead. Verified: three windows on a server capped at 100 held
  it at 35 files; windows 2 and 3 polled with live previews and no ✕.
- Banner (the maintenance slot, yellow): the server is refusing clients
  (from "server exited unexpectedly" on the subprocess fallbacks), sessions
  are going without pipes, or another window won't fit.
- The three pipe bugs above.
- Tests: the tmux, session and ui packages run against a private server.
  They used to create and kill sessions on the user's default server.

## Not done: moving the socket

#4 proposed a durable socket under `~/.agent-desk/tmux`. The evidence points
elsewhere (the cleaner can't remove a socket), and the move has a real
migration cost: clients pointed at a new socket can't see sessions on a
server already running at the old one. Left for a decision if the incident
recurs with a server whose limit is known to be high.

## For an existing server

A server started before this version keeps its limit until it exits. Closing
a window frees room at once. Lifting the limit means a new server, and ending
the old one ends its sessions (Claude ones resume with `R`). That is the
user's call; agent-desk never kills the server. See troubleshooting, "tmux
File Limit".
