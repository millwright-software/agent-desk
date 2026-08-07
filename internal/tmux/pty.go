//go:build !windows
// +build !windows

package tmux

import (
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

// Attach attaches to the tmux session with full PTY support
// Ctrl+Q will detach and return to the caller
func (s *Session) Attach(ctx context.Context) error {
	if !s.Exists() {
		return fmt.Errorf("session %s does not exist", s.Name)
	}

	// Create context with cancel for Ctrl+Q detach
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start tmux attach command with PTY
	cmd := exec.CommandContext(ctx, "tmux", "attach-session", "-t", s.Name)

	// Start command with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	// Save original terminal state and set raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
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
		return fmt.Errorf("failed to create cancelable stdin reader: %w", err)
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
	return result
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
