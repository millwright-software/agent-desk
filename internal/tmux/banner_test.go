//go:build !windows

package tmux

import (
	"strings"
	"testing"
)

func TestBannerSize(t *testing.T) {
	tests := []struct {
		name  string
		b     AttachBanner
		cols  int
		wantW int
		wantH int
	}{
		// "x" in the block font is 4 cells + 8 padding = 12, under the minimum.
		{"short title gets the minimum width, art height", AttachBanner{Title: "x"}, 120, bannerMinWidth, bannerArtHeight},
		// Nine four-wide glyphs and a three-wide I, nine gaps: 48, plus padding.
		{"art width follows the letters", AttachBanner{Title: "abcdefghij"}, 120, 56, bannerArtHeight},
		{"subtitle can widen the art card", AttachBanner{Title: "ab", Subtitle: strings.Repeat("g", 40)}, 120, 48, bannerArtHeight},
		// 20 letters of art is ~99 cells; an 80-col terminal cannot hold it, so
		// the card is the plain one-line kind, sized to the plain title.
		{"too wide for art falls back to plain", AttachBanner{Title: strings.Repeat("a", 20)}, 80, 28, bannerPlainHeight},
		{"plain card clamped to the client", AttachBanner{Title: strings.Repeat("a", 200)}, 80, 76, bannerPlainHeight},
		{"unknown client size does not clamp", AttachBanner{Title: strings.Repeat("a", 60)}, 0, 60*5 - 1 + bannerPadding, bannerArtHeight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := bannerSize(tt.b, tt.cols)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("bannerSize = %dx%d, want %dx%d", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

// The parent sizes the popup; the card inside renders from the interior it
// finds. Whatever the title and terminal, both must land on the same kind of
// card, or the letters get clipped (art in a plain box) or the box is mostly
// empty (plain text in an art box).
func TestBannerSizeAndRenderAgree(t *testing.T) {
	for _, title := range []string{"x", "agent-desk", "millwright-software", strings.Repeat("w", 30), "héllo wörld", "a b"} {
		for _, cols := range []int{40, 80, 120, 200} {
			w, h := bannerSize(AttachBanner{Title: title, Subtitle: "grp"}, cols)
			out := RenderBanner(title, "grp", w-2, h-2)
			lines := strings.Split(out, "\r\n")
			gotArt := len(lines) > 3
			wantArt := h == bannerArtHeight
			if gotArt != wantArt {
				t.Errorf("title %q at %d cols: popup %dx%d (art=%v) but renderer drew %d lines (art=%v)", title, cols, w, h, wantArt, len(lines), gotArt)
			}
			if len(lines) > h-2 {
				t.Errorf("title %q at %d cols: %d lines do not fit interior height %d", title, cols, len(lines), h-2)
			}
		}
	}
}

func TestRenderBlockText(t *testing.T) {
	rows := renderBlockText("Ab-1")
	if len(rows) != bannerFontRows {
		t.Fatalf("want %d rows, got %d", bannerFontRows, len(rows))
	}
	w := len([]rune(rows[0]))
	for i, r := range rows {
		if len([]rune(r)) != w {
			t.Errorf("row %d width %d, want %d (rows must be rectangular):\n%s", i, len([]rune(r)), w, strings.Join(rows, "\n"))
		}
	}
	// A(4) + gap + B(4) + gap + -(3) + gap + 1(3)
	if w != 4+1+4+1+3+1+3 {
		t.Errorf("width %d", w)
	}
	// Every glyph in the font is rectangular too.
	for r, g := range bannerFont {
		for i := 1; i < bannerFontRows; i++ {
			if len([]rune(g[i])) != len([]rune(g[0])) {
				t.Errorf("glyph %q row %d is %d wide, row 0 is %d", r, i, len([]rune(g[i])), len([]rune(g[0])))
			}
		}
	}
	// Unknown characters are drawn as themselves rather than dropped.
	if rows := renderBlockText("é"); rows[2] != "é" {
		t.Errorf("unknown rune should appear on the middle row, got %q", rows)
	}
}

func TestIsTerminalResponse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"DA1 reply", "\x1b[?62;22c", true},
		{"DA2 reply", "\x1b[>1;95;0c", true},
		{"cursor position report", "\x1b[24;80R", true},
		{"device status ok", "\x1b[0n", true},
		{"OSC colour reply", "\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\", true},
		{"DCS XTVERSION reply", "\x1bP>|tmux 3.6a\x1b\\", true},
		{"kitty keyboard flags reply", "\x1b[?0u", true},

		{"a letter", "a", false},
		{"enter", "\r", false},
		{"up arrow", "\x1b[A", false},
		{"shift+right", "\x1b[1;2C", false},
		{"F5", "\x1b[15~", false},
		{"alt+x", "\x1bx", false},
		{"bare escape", "\x1b", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTerminalResponse([]byte(tt.in)); got != tt.want {
				t.Errorf("IsTerminalResponse(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// The popup must exec the binary directly with the text in the environment.
// A title with quotes or a `;` must never reach a shell.
func TestBannerPopupArgsNeverShellQuotes(t *testing.T) {
	b := AttachBanner{Title: `it's; "weird" $(x)`, Subtitle: "grp/sub"}
	args := bannerPopupArgs("/dev/ttys004", "agent-desk_abc", "/usr/local/bin/agent-desk", b, 100)

	joined := strings.Join(args, "\x00")
	for _, want := range []string{
		"-c\x00/dev/ttys004",
		"-t\x00agent-desk_abc",
		"-e\x00" + BannerEnvTitle + "=" + b.Title,
		"-e\x00" + BannerEnvSubtitle + "=grp/sub",
		"-e\x00" + BannerEnvTarget + "=agent-desk_abc",
		"/usr/local/bin/agent-desk\x00" + BannerSubcommand,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q:\n%q", want, args)
		}
	}
	// Last two args are exe + verb, which is what makes tmux exec instead of `sh -c`.
	if args[len(args)-1] != BannerSubcommand {
		t.Errorf("last arg = %q, want the subcommand", args[len(args)-1])
	}
}

func TestRenderBanner(t *testing.T) {
	// Plain card: a 3-row interior is too short for the block font.
	out := RenderBanner("work", "grp", 20, 3)
	lines := strings.Split(out, "\r\n")
	if len(lines) != 3 || lines[0] != "" {
		t.Fatalf("want blank line then two lines, got %q", lines)
	}
	if !strings.Contains(lines[1], "\x1b[1m"+strings.Repeat(" ", 8)+"work") {
		t.Errorf("title not bold and centred: %q", lines[1])
	}
	if !strings.Contains(lines[2], "\x1b[2m") || !strings.Contains(lines[2], "grp") {
		t.Errorf("subtitle not dimmed: %q", lines[2])
	}

	// Art card: blank, five letter rows, blank, subtitle.
	art := strings.Split(RenderBanner("work", "grp", 40, bannerArtHeight-2), "\r\n")
	if len(art) != 1+bannerFontRows+1+1 {
		t.Fatalf("art card should be %d lines, got %d:\n%s", 1+bannerFontRows+2, len(art), strings.Join(art, "\n"))
	}
	if !strings.Contains(art[1], "█") || !strings.Contains(art[len(art)-1], "grp") {
		t.Errorf("art card lacks letters or subtitle:\n%s", strings.Join(art, "\n"))
	}

	// No subtitle: no dimmed line at all, not an empty one.
	if got := strings.Count(RenderBanner("work", "", 20, 3), "\r\n"); got != 1 {
		t.Errorf("without subtitle want 1 line break, got %d", got)
	}

	// Too long for the box: truncated with an ellipsis, never wrapped.
	long := RenderBanner(strings.Repeat("a", 50), "", 10, 3)
	for _, l := range strings.Split(long, "\r\n") {
		if strings.Count(l, "a") > 10 {
			t.Errorf("line not truncated to 10 cells: %q", l)
		}
	}
	if !strings.Contains(long, "…") {
		t.Errorf("truncation should show an ellipsis: %q", long)
	}

	// A one-row popup interior shows the title and nothing else.
	if got := RenderBanner("t", "s", 20, 1); strings.Contains(got, "\r\n") {
		t.Errorf("one row should be one line: %q", got)
	}
}

func TestParseClientByPID(t *testing.T) {
	out := "/dev/ttys001 4242\n/dev/ttys007 5150\n\n"
	if got := parseClientByPID(out, 5150); got != "/dev/ttys007" {
		t.Errorf("got %q", got)
	}
	if got := parseClientByPID(out, 1); got != "" {
		t.Errorf("unknown pid should be empty, got %q", got)
	}
}
