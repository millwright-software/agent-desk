//go:build !windows

package tmux

import "testing"

func TestInputClassifierSingleChunks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"DA1 reply", "\x1b[?62;22c", false},
		{"DA2 reply", "\x1b[>1;95;0c", false},
		{"DA3 reply", "\x1b[=0c", false},
		{"cursor position report", "\x1b[24;80R", false},
		{"device status ok", "\x1b[0n", false},
		{"OSC colour reply, ST", "\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\", false},
		{"OSC colour reply, BEL", "\x1b]11;rgb:1e1e/1e1e/2e2e\x07", false},
		{"DCS XTVERSION reply", "\x1bP>|tmux 3.6a\x1b\\", false},
		{"kitty keyboard flags reply", "\x1b[?0u", false},
		{"focus in", "\x1b[I", false},
		{"focus out", "\x1b[O", false},
		{"SGR mouse press", "\x1b[<0;10;5M", false},
		{"SGR mouse release", "\x1b[<0;10;5m", false},
		{"three replies in one read", "\x1b[?62;22c\x1b[>1;95;0c\x1bP>|tmux 3.6a\x1b\\", false},

		{"a letter", "a", true},
		{"enter", "\r", true},
		{"ctrl+c", "\x03", true},
		{"up arrow", "\x1b[A", true},
		{"shift+right", "\x1b[1;2C", true},
		{"home", "\x1b[H", true},
		{"F5", "\x1b[15~", true},
		{"delete", "\x1b[3~", true},
		{"F1 as SS3", "\x1bOP", true},
		{"application-mode up", "\x1bOA", true},
		{"alt+x", "\x1bx", true},
		{"escape escape", "\x1b\x1b", true},
		{"reply then a letter", "\x1b[?62;22cq", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c InputClassifier
			if got := c.Feed([]byte(tt.in)); got != tt.want {
				t.Errorf("Feed(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ⚠️ The case that produced the bug: replies arriving across reads. Judged
// chunk by chunk, the second read starts with an ordinary byte.
func TestInputClassifierSplitAcrossReads(t *testing.T) {
	var c InputClassifier
	chunks := []string{
		"\x1b[?62;22c\x1b[>1;95;0c\x1bP>|tm", // 32 bytes, as the old read size cut it
		"ux 3.6a\x1b\\\x1b]11;rgb:1e",
		"1e/1e1e/2e2e\x1b\\",
	}
	for i, ch := range chunks {
		if c.Feed([]byte(ch)) {
			t.Fatalf("chunk %d %q was taken for typing", i, ch)
		}
	}
	// And after all that, a real key is still recognised.
	if !c.Feed([]byte("j")) {
		t.Error("a key after the replies should count")
	}

	// A key sequence split across reads still counts, once complete.
	var d InputClassifier
	if d.Feed([]byte("\x1b")) {
		t.Error("a lone ESC is not yet a key")
	}
	if !d.Feed([]byte("[A")) {
		t.Error("completing the arrow should count")
	}
}

// A bare Escape press followed by nothing: the parser waits for more, and
// the next plain byte is read as Alt+key, which is a key either way.
func TestInputClassifierLoneEscapeThenLetter(t *testing.T) {
	var c InputClassifier
	c.Feed([]byte("\x1b"))
	if !c.Feed([]byte("a")) {
		t.Error("ESC then a should be Alt+a, a key")
	}
}
