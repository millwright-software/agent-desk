//go:build !windows
// +build !windows

package tmux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

// Scrollback-clear escape sequences (ported from upstream agent-deck #419/#618).
const (
	// clearScrollbackCSI is CSI 3 J — "Erase Saved Lines". Honored by Terminal.app,
	// WezTerm, Alacritty, Ghostty, Kitty, xterm, and older iTerm2 builds.
	clearScrollbackCSI = "\x1b[3J"
	// itermClearScrollback is OSC 1337 ; ClearScrollback BEL — iTerm2-specific.
	// Required by iTerm2 3.6.x when "Save lines to scrollback in alternate
	// screen mode" is OFF. Other terminals parse the OSC payload and discard it
	// safely — adding this escape is strictly additive.
	itermClearScrollback = "\x1b]1337;ClearScrollback\a"
)

// emitScrollbackClear writes escape sequences to clear the host terminal's
// scrollback buffer. Both the generic CSI 3 J escape AND the iTerm2-specific
// OSC 1337 ClearScrollback escape are emitted. Both the attach entry and the
// detach exit route through this helper so the two boundaries cannot drift.
func emitScrollbackClear(w io.Writer) {
	_, _ = io.WriteString(w, clearScrollbackCSI)
	_, _ = io.WriteString(w, itermClearScrollback)
}

// StartAttachPTY starts cmd attached to a new PTY pre-sized to tty's current
// dimensions (ported from upstream agent-deck #1167).
//
// tmux clients connect at their PTY's size. A bare pty.Start creates the
// attach client's PTY at the 80x24 default, so the window snaps to 80 cols
// until the async SIGWINCH grows it — and each of those resizes makes the
// inner tool fully repaint, pushing wrong-width duplicate frames into
// scrollback. Reading the controlling terminal's real size up front and
// starting the PTY with it makes the client full-size from frame one.
//
// When tty is not a terminal (size probe fails), it falls back to a plain
// start at the default size: a degraded attach is still better than no attach.
func StartAttachPTY(cmd *exec.Cmd, tty *os.File) (*os.File, error) {
	if tty != nil {
		if ws, err := pty.GetsizeFull(tty); err == nil && ws.Cols > 0 && ws.Rows > 0 {
			return pty.StartWithSize(cmd, ws)
		}
	}
	return pty.Start(cmd)
}

// SwitchDirection is what the user asked for on the way out of an attach.
// SwitchNone means an ordinary detach — Ctrl+Q, tmux's own Ctrl+B d, or the
// session's command exiting.
type SwitchDirection int

const (
	SwitchNone SwitchDirection = iota
	SwitchNext
	SwitchPrev
)

// ⚠️ SHIFT+ARROWS, NOT SHIFT+BRACKETS. In a terminal Shift+] IS "}" and
// Shift+[ IS "{" — they are the ASCII bytes 0x7D/0x7B, not distinct keys.
// Intercepting those mid-attach would eat every brace typed into the session,
// which in a coding tool is not a trade worth making. Shift+Right/Left send
// dedicated CSI sequences that nothing types by accident.
//
// ⚠️ They arrive as a multi-byte read, so they cannot be matched the way Ctrl+Q
// is (n == 1 && buf[0] == 0x11). They are compared against the whole slice, and
// a short read that splits the sequence just forwards it — the safe failure,
// because the session then sees an ordinary Shift+Arrow.
var (
	seqShiftRight = []byte{0x1b, '[', '1', ';', '2', 'C'}
	seqShiftLeft  = []byte{0x1b, '[', '1', ';', '2', 'D'}
)

// switchForBytes decides whether one raw stdin read is a switch request.
//
// The match is against the WHOLE read, never a prefix or a substring: pasted
// text can contain anything, and a paste that happened to include these six
// bytes must not yank the user into another session.
func switchForBytes(b []byte) SwitchDirection {
	switch {
	case bytes.Equal(b, seqShiftRight):
		return SwitchNext
	case bytes.Equal(b, seqShiftLeft):
		return SwitchPrev
	}
	return SwitchNone
}

// Attach attaches to the tmux session with full PTY support.
// Ctrl+Q detaches and returns to the caller.
func (s *Session) Attach(ctx context.Context) error {
	_, err := s.AttachSwitchable(ctx, nil)
	return err
}

// AttachSwitchable is Attach, and additionally reports whether the user asked
// to move to the next or previous session on the way out (Shift+Right /
// Shift+Left). It only reports intent; the caller decides what "next" means.
//
// banner, when non-nil, is shown over the session for a moment on landing
// (see AttachBanner). The TUI passes one when the attach is the result of a
// switch, where you did not pick the session from a list and may not know
// which one you are looking at.
//
// The plain Attach wrapper stays because tea.ExecCommand fixes Run() error, and
// the CLI's `session attach` has no session ring to move around in.
func (s *Session) AttachSwitchable(ctx context.Context, banner *AttachBanner) (SwitchDirection, error) {
	switchTo := SwitchNone
	if !s.Exists() {
		return switchTo, fmt.Errorf("session %s does not exist", s.Name)
	}

	// Clear the outer terminal emulator's scrollback buffer to prevent stale
	// frames from a previously-attached session (or a previous attach of this
	// one) bleeding into what the user sees when they scroll up (#419/#618).
	//
	// Note: we intentionally do NOT call `tmux clear-history` here. tmux pane
	// histories are per-pane; clearing them on attach would destroy the user's
	// scrollback and break mouse-wheel / copy-mode navigation (#531).
	emitScrollbackClear(os.Stdout)

	// Create context with cancel for Ctrl+Q detach
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start tmux attach command with PTY
	cmd := exec.CommandContext(ctx, "tmux", "attach-session", "-t", s.Name)

	// Start command with PTY, pre-sized to the controlling terminal so the
	// tmux client connects at full size from frame one (#1167).
	ptmx, err := StartAttachPTY(cmd, os.Stdin)
	if err != nil {
		return switchTo, fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	// Save original terminal state and set raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return switchTo, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Cancelable stdin reader for the input goroutine below. Without it, that
	// goroutine stays blocked in a raw os.Stdin.Read; on a tmux-native detach
	// (Ctrl+b d) or when the session's command exits, Attach returns while the
	// reader is still parked, orphaning it. The orphan then consumes the FIRST
	// keystroke after control returns to the TUI (the reported "d does nothing
	// until I switch sessions and back" bug). cancelreader interrupts the read
	// cleanly and handles macOS TTYs (kqueue) — unlike os.Stdin.SetReadDeadline,
	// which the Go poller does not honor on darwin terminals.
	stdinReader, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return switchTo, fmt.Errorf("failed to create cancelable stdin reader: %w", err)
	}

	// Handle window resize signals
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	sigwinchDone := make(chan struct{}) // Signal for SIGWINCH goroutine to exit
	defer func() {
		signal.Stop(sigwinch)
		close(sigwinchDone) // Signal goroutine to exit
		// Don't close sigwinch - signal.Stop() handles cleanup
	}()

	// WaitGroup tracks the output/SIGWINCH/cmd goroutines. The stdin reader is
	// tracked separately (stdinDone) because it must be joined before return.
	var wg sync.WaitGroup

	// SIGWINCH handler goroutine - properly tracked in WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-sigwinchDone:
				return
			case _, ok := <-sigwinch:
				if !ok {
					return
				}
				if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
					_ = pty.Setsize(ptmx, ws)
				}
			}
		}
	}()
	// Initial resize
	sigwinch <- syscall.SIGWINCH

	// Landing card. Opened from a goroutine once the tmux client registers;
	// the first keystroke below dismisses it so nothing typed is swallowed.
	var popup *bannerPopup
	if banner != nil {
		if exe, err := os.Executable(); err == nil {
			cols := 0
			if ws, err := pty.GetsizeFull(os.Stdin); err == nil {
				cols = int(ws.Cols)
			}
			popup = &bannerPopup{sessionName: s.Name, clientPID: cmd.Process.Pid, exe: exe, banner: *banner, cols: cols}
			go popup.show(ctx)
		}
	}

	// Channel to signal detach via Ctrl+Q
	detachCh := make(chan struct{})

	// Channel for I/O errors (buffered to prevent goroutine leaks)
	ioErrors := make(chan error, 2)

	// Timeout to ignore initial terminal control sequences (50ms)
	startTime := time.Now()
	const controlSeqTimeout = 50 * time.Millisecond

	// Goroutine 1: Copy PTY output to stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := io.Copy(os.Stdout, ptmx)
		if err != nil && err != io.EOF {
			// Only report non-EOF errors (EOF is normal on PTY close)
			select {
			case ioErrors <- fmt.Errorf("PTY read error: %w", err):
			default:
				// Channel full, error already reported
			}
		}
	}()

	// Goroutine 2: Read stdin, intercept Ctrl+Q (0x11), forward rest to PTY.
	// Joined explicitly via stdinDone (not wg) so we can guarantee it has fully
	// exited before Attach returns and hands stdin back to the TUI.
	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		buf := make([]byte, 32)
		for {
			n, err := stdinReader.Read(buf)
			if err != nil {
				// Canceled (on return), EOF, or a real read error — stop.
				if err != cancelreader.ErrCanceled && err != io.EOF {
					select {
					case ioErrors <- fmt.Errorf("stdin read error: %w", err):
					default:
					}
				}
				return
			}

			// Discard initial terminal control sequences (within first 50ms)
			// These are things like terminal capability queries
			if time.Since(startTime) < controlSeqTimeout {
				continue
			}

			// Check for Ctrl+Q (0x11 = DC1/XON) - single byte, reliable in raw mode
			if n == 1 && buf[0] == 0x11 {
				close(detachCh)
				cancel()
				return
			}

			// Shift+Right / Shift+Left: detach and ask the caller to move to the
			// next or previous session. ⚠️ Written BEFORE detachCh closes — the
			// select below returns as soon as it does, so a write afterwards
			// races the caller's read and the switch is silently dropped.
			if dir := switchForBytes(buf[:n]); dir != SwitchNone {
				switchTo = dir
				close(detachCh)
				cancel()
				return
			}

			// A real keystroke: take the landing card down first so the key
			// lands in the session, not in the card.
			if popup != nil {
				popup.dismiss()
			}

			// Forward other input to tmux PTY
			if _, err := ptmx.Write(buf[:n]); err != nil {
				// Report PTY write error
				select {
				case ioErrors <- fmt.Errorf("PTY write error: %w", err):
				default:
				}
				return
			}
		}
	}()

	// Wait for command to finish - tracked in WaitGroup
	cmdDone := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmdDone <- cmd.Wait()
	}()

	// Wait for either detach (Ctrl+Q) or command completion
	var result error
	select {
	case <-detachCh:
		// User pressed Ctrl+Q, detach gracefully
		result = nil
	case err := <-cmdDone:
		if err != nil {
			// Normal tmux detach (Ctrl+B,D) exits 0/1; Ctrl+Q cancels the context.
			if exitErr, ok := err.(*exec.ExitError); ok &&
				(exitErr.ExitCode() == 0 || exitErr.ExitCode() == 1) {
				err = nil
			} else if ctx.Err() != nil {
				err = nil
			}
		}
		result = err
	case <-ctx.Done():
		result = nil
	}

	// Stop and join the stdin reader BEFORE the deferred term.Restore hands
	// stdin back to the TUI — otherwise the orphaned reader swallows the next
	// keystroke. Cancel() unblocks an in-flight Read; stdinDone confirms exit.
	stdinReader.Cancel()
	<-stdinDone
	_ = stdinReader.Close()

	// Clear host terminal scrollback before returning to the TUI. The on-attach
	// clear at the top of Attach() covers the "next attach" direction; this
	// covers the "on detach" direction so stale frames of the session never sit
	// in the host scrollback behind the TUI (#419/#618).
	emitScrollbackClear(os.Stdout)
	return switchTo, result
}

// Resize changes the terminal size of the tmux session
func (s *Session) Resize(cols, rows int) error {
	// Resize the tmux window
	cmd := exec.Command("tmux", "resize-window", "-t", s.Name, "-x", fmt.Sprintf("%d", cols), "-y", fmt.Sprintf("%d", rows))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to resize window: %w", err)
	}
	return nil
}

// AttachReadOnly attaches to the session in read-only mode
func (s *Session) AttachReadOnly(ctx context.Context) error {
	if !s.Exists() {
		return fmt.Errorf("session %s does not exist", s.Name)
	}

	// Save original terminal state
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Start tmux attach command in read-only mode
	cmd := exec.CommandContext(ctx, "tmux", "attach-session", "-r", "-t", s.Name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the attach command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to attach to session: %w", err)
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		// Check if it's a normal detach
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 || exitErr.ExitCode() == 1 {
				return nil
			}
		}
		return fmt.Errorf("attach command failed: %w", err)
	}

	return nil
}

// StreamOutput streams the session output to the provided writer
func (s *Session) StreamOutput(ctx context.Context, w io.Writer) error {
	if !s.Exists() {
		return fmt.Errorf("session %s does not exist", s.Name)
	}

	// Use tmux pipe-pane to stream output
	cmd := exec.CommandContext(ctx, "tmux", "pipe-pane", "-t", s.Name, "-o", "cat")
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pipe-pane: %w", err)
	}

	// Wait for context cancellation or command completion
	// Use WaitGroup to prevent goroutine leak on context cancellation
	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errChan <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Stop pipe-pane - error is intentionally ignored since we're
		// already returning ctx.Err() and cleanup failure is non-fatal
		stopCmd := exec.Command("tmux", "pipe-pane", "-t", s.Name)
		_ = stopCmd.Run()
		// Wait for the goroutine to complete before returning
		wg.Wait()
		return ctx.Err()
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("pipe-pane failed: %w", err)
		}
		return nil
	}
}
