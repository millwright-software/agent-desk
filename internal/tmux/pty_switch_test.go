//go:build !windows

package tmux

import "testing"

// What the attach loop does with one raw stdin read.
func TestSwitchForBytes(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want SwitchDirection
	}{
		{"shift+right", []byte("\x1b[1;2C"), SwitchNext},
		{"shift+left", []byte("\x1b[1;2D"), SwitchPrev},

		// ⚠️ THE WHOLE REASON THIS IS NOT BOUND TO SHIFT+BRACKETS. Shift+] IS
		// "}" and Shift+[ IS "{". Swallowing those would make the editor inside
		// every attached session unusable, which is not a trade worth making in
		// a tool for writing code.
		{"a closing brace is just a brace", []byte("}"), SwitchNone},
		{"an opening brace is just a brace", []byte("{"), SwitchNone},
		{"a bare bracket", []byte("]"), SwitchNone},

		// Plain arrows must reach the session — they are how you move a cursor.
		{"plain right arrow", []byte("\x1b[C"), SwitchNone},
		{"plain left arrow", []byte("\x1b[D"), SwitchNone},
		{"application-mode right", []byte("\x1bOC"), SwitchNone},

		// Other modifiers are not ours. Ctrl+Right is word-motion in most shells.
		{"ctrl+right", []byte("\x1b[1;5C"), SwitchNone},
		{"alt+right", []byte("\x1b[1;3C"), SwitchNone},
		{"shift+up is not a switch", []byte("\x1b[1;2A"), SwitchNone},
		{"shift+down is not a switch", []byte("\x1b[1;2B"), SwitchNone},

		// Ctrl+Q keeps its own single-byte path; it must not match here.
		{"ctrl+q", []byte{0x11}, SwitchNone},
		{"escape alone", []byte{0x1b}, SwitchNone},
		{"empty read", []byte{}, SwitchNone},

		// ⚠️ A PASTE CONTAINING THE SEQUENCE MUST NOT SWITCH. This is why the
		// match is whole-read equality rather than a prefix or substring test.
		{"paste containing the sequence", []byte("x\x1b[1;2Cy"), SwitchNone},
		{"sequence with a trailing byte", []byte("\x1b[1;2CC"), SwitchNone},
		{"sequence with a leading byte", []byte("a\x1b[1;2C"), SwitchNone},

		// A split read forwards rather than switching — the safe failure.
		{"first half of the sequence", []byte("\x1b[1;"), SwitchNone},
		{"second half of the sequence", []byte("2C"), SwitchNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := switchForBytes(tc.in); got != tc.want {
				t.Errorf("switchForBytes(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// The two sequences are the terminfo kRIT/kLFT of xterm-256color, which is what
// iTerm2, Terminal.app, Ghostty, WezTerm and Alacritty all report.
func TestSwitchSequencesMatchTerminfo(t *testing.T) {
	if string(seqShiftRight) != "\x1b[1;2C" {
		t.Errorf("shift+right = %q, want xterm kRIT \\E[1;2C", seqShiftRight)
	}
	if string(seqShiftLeft) != "\x1b[1;2D" {
		t.Errorf("shift+left = %q, want xterm kLFT \\E[1;2D", seqShiftLeft)
	}
}
