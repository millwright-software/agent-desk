package tmux

// Regression tests for the scrollback/wrap fixes ported from upstream
// agent-deck: #1694 (birth detached sessions at the real terminal size),
// #1167 (pre-size the attach PTY), and #419/#618 (clear host terminal
// scrollback on attach/detach, including the iTerm2 OSC 1337 escape).

import (
	"strings"
	"testing"
)

// pinTerminalSizeProbe swaps the probe seam for the duration of a test.
func pinTerminalSizeProbe(t *testing.T, cols, rows int, ok bool) {
	t.Helper()
	orig := terminalSizeProbe
	terminalSizeProbe = func() (int, int, bool) { return cols, rows, ok }
	t.Cleanup(func() { terminalSizeProbe = orig })
}

func TestInitialWindowSize_UsesRealTerminalSize(t *testing.T) {
	pinTerminalSizeProbe(t, 173, 41, true)
	cols, rows := InitialWindowSize()
	if cols != 173 || rows != 41 {
		t.Fatalf("expected 173x41, got %dx%d", cols, rows)
	}
}

func TestInitialWindowSize_FlooredAtTmuxDefault(t *testing.T) {
	pinTerminalSizeProbe(t, 40, 10, true)
	cols, rows := InitialWindowSize()
	if cols != tmuxDefaultCols || rows != tmuxDefaultRows {
		t.Fatalf("expected floor %dx%d, got %dx%d", tmuxDefaultCols, tmuxDefaultRows, cols, rows)
	}
}

func TestInitialWindowSize_HeadlessFallback(t *testing.T) {
	pinTerminalSizeProbe(t, 0, 0, false)
	cols, rows := InitialWindowSize()
	if cols != headlessInitialCols || rows != headlessInitialRows {
		t.Fatalf("expected headless %dx%d, got %dx%d", headlessInitialCols, headlessInitialRows, cols, rows)
	}
}

func TestEmitScrollbackClear_IncludesBothEscapes(t *testing.T) {
	var b strings.Builder
	emitScrollbackClear(&b)
	out := b.String()
	if !strings.Contains(out, clearScrollbackCSI) {
		t.Errorf("missing CSI 3 J escape in %q", out)
	}
	if !strings.Contains(out, itermClearScrollback) {
		t.Errorf("missing iTerm2 OSC 1337 ClearScrollback escape in %q", out)
	}
	if !strings.HasPrefix(out, clearScrollbackCSI) {
		t.Errorf("CSI escape must come first (broad compatibility) in %q", out)
	}
}
