package tmux

import (
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The tmux server's open-file limit is the ceiling on everything agent-desk
// does with it. Each control-mode client (one per session per agent-desk
// window) costs the server three descriptors: its socket plus the stdin and
// stdout it passes over. At the limit tmux cannot take a new client and every
// command from every window fails with "server exited unexpectedly", though
// the server and its agents are still running (issues #3, #4).
//
// macOS gives processes a soft limit of 256. Go raises its own limit at
// startup but hands the original 256 back to every child it execs, so a tmux
// server agent-desk started was capped at 256: about 65 clients, three or
// four windows on twenty sessions.

// fileLimitTarget is the soft limit agent-desk asks for, and so the limit of
// any tmux server it starts: room for about 5000 control clients.
const fileLimitTarget = 16384

// RaiseFileLimit sets this process's soft open-file limit to fileLimitTarget
// (bounded by the hard limit) before any tmux command runs. Setting it
// explicitly is what makes children inherit it instead of the original.
// Returns the soft limit in force afterwards.
func RaiseFileLimit() (uint64, error) {
	var r syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &r); err != nil {
		return 0, err
	}
	target := uint64(fileLimitTarget)
	if r.Max < target {
		target = r.Max
	}
	r.Cur = target
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &r); err != nil {
		return 0, err
	}
	return r.Cur, nil
}

// ServerFiles is the running tmux server's open-file limit and use.
type ServerFiles struct {
	PID   int
	Limit int // soft RLIMIT_NOFILE of the server process
	InUse int // descriptors it holds now
}

// Headroom is how many descriptors the server has left.
func (f ServerFiles) Headroom() int { return f.Limit - f.InUse }

// Low reports a server whose limit is the old default class (under 1024),
// the ones a few agent-desk windows can fill.
func (f ServerFiles) Low() bool { return f.Limit > 0 && f.Limit < 1024 }

// Relief is the shortest instruction that gives the server room again. A
// server on the old default class (Low) stays capped however many windows
// close, so closing one only buys time; the real fix is to restart it so a
// fresh server inherits agent-desk's raised limit. That restart ends every
// session in the server, so the instruction says so: the banner must not
// send someone to the destructive fix without its cost. A roomier server
// that is merely full answers again the moment a window closes.
func (f ServerFiles) Relief() string {
	if f.Low() {
		return "Only a tmux server restart lifts its limit, and that ends every session (R resumes Claude ones). Troubleshooting: tmux file limit"
	}
	return "Close an extra agent-desk window (Troubleshooting: tmux file limit)"
}

// ProbeServerFiles reads the default tmux server's limit (through a shell it
// spawns, which inherits it) and counts its descriptors with lsof.
func ProbeServerFiles() (ServerFiles, error) {
	var f ServerFiles
	out, err := exec.Command("tmux", "display-message", "-p", "#{pid}").Output()
	noteTmuxResult(err, exitStderr(err))
	if err != nil {
		return f, fmt.Errorf("tmux server pid: %w", err)
	}
	if f.PID, err = strconv.Atoi(strings.TrimSpace(string(out))); err != nil {
		return f, fmt.Errorf("tmux server pid %q: %w", out, err)
	}
	out, err = exec.Command("tmux", "run-shell", "ulimit -n").Output()
	if err != nil {
		return f, fmt.Errorf("tmux server limit: %w", err)
	}
	if s := strings.TrimSpace(string(out)); s == "unlimited" {
		f.Limit = 1 << 30
	} else if f.Limit, err = strconv.Atoi(s); err != nil {
		return f, fmt.Errorf("tmux server limit %q: %w", s, err)
	}
	out, err = exec.Command("lsof", "-p", strconv.Itoa(f.PID), "-F", "f").Output()
	if err != nil && len(out) == 0 {
		return f, fmt.Errorf("lsof tmux server: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "f") {
			f.InUse++
		}
	}
	return f, nil
}

// pipeFDCost is what one control-mode client costs the server.
const pipeFDCost = 3

// pipeFDReserve is held back from pipes for everything else: the transient
// commands agent-desk runs (each a client while it lives, ten at a time from
// the status worker), attaches, and new sessions' panes.
const pipeFDReserve = 64

// PipeBudget is how many more control pipes the server can take without
// eating the reserve. Zero means connect none.
func (f ServerFiles) PipeBudget() int {
	n := (f.Headroom() - pipeFDReserve) / pipeFDCost
	if n < 0 {
		return 0
	}
	return n
}

// --- unreachable server ---

var (
	unreachableMu    sync.RWMutex
	unreachableSince time.Time
)

// noteTmuxResult records whether a tmux command reached the server. The
// signature of a full server is "server exited unexpectedly" from a client
// while the server is still alive, not "no server running".
func noteTmuxResult(err error, stderr []byte) {
	full := err != nil && strings.Contains(string(stderr), "server exited unexpectedly")
	unreachableMu.Lock()
	defer unreachableMu.Unlock()
	switch {
	case full && unreachableSince.IsZero():
		unreachableSince = time.Now()
		statusLog.Warn("tmux_server_unreachable", slog.String("stderr", strings.TrimSpace(string(stderr))))
	case err == nil && !unreachableSince.IsZero():
		statusLog.Info("tmux_server_reachable_again", slog.Duration("after", time.Since(unreachableSince)))
		unreachableSince = time.Time{}
	}
}

// exitStderr is the stderr exec.Cmd.Output captured on a failed command.
func exitStderr(err error) []byte {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.Stderr
	}
	return nil
}

// ServerUnreachable reports whether the tmux server is refusing clients:
// alive (it answered before) but out of descriptors. Since when, if so.
func ServerUnreachable() (time.Time, bool) {
	unreachableMu.RLock()
	defer unreachableMu.RUnlock()
	return unreachableSince, !unreachableSince.IsZero()
}
