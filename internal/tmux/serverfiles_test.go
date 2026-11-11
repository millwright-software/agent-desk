package tmux

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The point of RaiseFileLimit is what children inherit: Go restores the
// original soft limit in every exec'd child unless the program set it.
func TestRaiseFileLimitReachesChildren(t *testing.T) {
	got, err := RaiseFileLimit()
	require.NoError(t, err)
	var r syscall.Rlimit
	require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_NOFILE, &r))
	want := min(uint64(fileLimitTarget), r.Max)
	assert.Equal(t, want, got)

	out, err := exec.Command("sh", "-c", "ulimit -n").Output()
	require.NoError(t, err)
	child, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	require.NoError(t, err)
	assert.Equal(t, want, child, "child process should inherit the raised limit")
}

func TestProbeServerFiles(t *testing.T) {
	skipIfNoTmuxServer(t)
	f, err := ProbeServerFiles()
	require.NoError(t, err)
	assert.Greater(t, f.PID, 0)
	assert.Greater(t, f.Limit, 0)
	assert.Greater(t, f.InUse, 0)
}

func TestPipeBudget(t *testing.T) {
	assert.Equal(t, 0, ServerFiles{Limit: 256, InUse: 250}.PipeBudget())
	assert.Equal(t, (256-117-pipeFDReserve)/pipeFDCost, ServerFiles{Limit: 256, InUse: 117}.PipeBudget())
	assert.True(t, ServerFiles{Limit: 256}.Low())
	assert.False(t, ServerFiles{Limit: 16384}.Low())
}

// A server without room for another pipe gets none: Connect refuses before
// spawning a client, and the session is counted as going without.
func TestConnectRefusedWithoutServerRoom(t *testing.T) {
	pm := NewPipeManager(t.Context(), nil)
	defer pm.Close()
	pm.probeRoom = func() (ServerFiles, error) {
		return ServerFiles{PID: 1, Limit: 256, InUse: 256 - pipeFDReserve - 2*pipeFDCost}, nil
	}

	// Budget 2: both debits succeed (the sessions don't exist, so the pipes
	// fail and credit their room back), so the budget is still 2 after.
	for _, name := range []string{"agentdesk_nope_a", "agentdesk_nope_b"} {
		err := pm.Connect(name)
		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrNoServerRoom), "had room: %v", err)
	}
	room, skipped := pm.Room()
	assert.Equal(t, 2, room.PipeBudget(), "failed pipes give their room back")
	assert.Equal(t, 0, skipped)

	pm.roomMu.Lock()
	pm.room.InUse += 2 * pipeFDCost // two other clients arrived
	pm.roomMu.Unlock()
	err := pm.Connect("agentdesk_nope_c")
	assert.ErrorIs(t, err, ErrNoServerRoom)
	_, skipped = pm.Room()
	assert.Equal(t, 1, skipped)
}

// A pipe that dies is reaped and actually reconnected by its watcher. The
// watcher used to mark the session "reconnecting", which turned its own
// Connect into a no-op: it logged success and left the session with no pipe.
func TestWatchPipeReconnects(t *testing.T) {
	name := createTestSession(t, "reconnect")
	pm := NewPipeManager(t.Context(), nil)
	defer pm.Close()
	require.NoError(t, pm.Connect(name))
	first := pm.GetPipe(name)
	require.NotNil(t, first)
	pid := first.cmd.Process.Pid

	// Kill the client out from under the pipe.
	require.NoError(t, syscall.Kill(pid, syscall.SIGKILL))

	deadline := time.Now().Add(6 * time.Second) // first retry at 2s
	for time.Now().Before(deadline) {
		if p := pm.GetPipe(name); p != nil && p != first && p.IsAlive() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	second := pm.GetPipe(name)
	require.NotNil(t, second, "watcher should have reconnected")
	assert.NotSame(t, first, second)
	assert.True(t, second.IsAlive())

	// The old client was reaped: no zombie left for its pid.
	var ws syscall.WaitStatus
	_, err := syscall.Wait4(pid, &ws, syscall.WNOHANG, nil)
	assert.Error(t, err, "old tmux -C should already be reaped (ECHILD)")
}

// A full server is recognized by its signature, and clears on success.
func TestNoteTmuxResult(t *testing.T) {
	defer func() { noteTmuxResult(nil, nil) }()
	noteTmuxResult(errors.New("exit 1"), []byte("no server running on /tmp/x\n"))
	_, full := ServerUnreachable()
	assert.False(t, full, "no server is not a full server")

	noteTmuxResult(errors.New("exit 1"), []byte("server exited unexpectedly\n"))
	_, full = ServerUnreachable()
	assert.True(t, full)

	noteTmuxResult(nil, nil)
	_, full = ServerUnreachable()
	assert.False(t, full)
}
