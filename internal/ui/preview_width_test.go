package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	runewidth "github.com/mattn/go-runewidth"
)

// ⚠️ THE DISAGREEMENT THIS FIX EXISTS FOR. go-runewidth and lipgloss measure a
// variation-selector emoji differently — "⚠️" is 1 cell to one and 2 to the
// other. The preview pane used to check its lines with runewidth while
// lipgloss.JoinHorizontal sized the panels, so a line runewidth judged to fit
// was a cell too wide for the join. The join then pads every line to the larger
// figure, the joined row runs past the terminal, it wraps, and the frame is one
// row taller than the screen — which shows up as the help bar drawn two to four
// times at the bottom.
func TestRunewidthAndLipglossStillDisagree(t *testing.T) {
	const s = "⚠️"
	rw, lw := runewidth.StringWidth(s), lipgloss.Width(s)
	if rw == lw {
		t.Skipf("the libraries now agree on %q (%d) — this fix has become belt-and-braces", s, lw)
	}
	t.Logf("confirmed: runewidth says %d, lipgloss says %d for %q", rw, lw, s)
}

// ⚠️ TRUNCATION MUST USE THE JOIN'S RULER. Whatever comes back must fit the
// budget as lipgloss measures it, because lipgloss is what lays it out.
func TestTruncateToWidthRespectsLipglossWidth(t *testing.T) {
	// Lines built from the characters that actually appear in captured agent
	// output — including the warning signs this codebase is full of.
	inputs := []string{
		strings.Repeat("a", 200),
		strings.Repeat("⚠️ warning ", 30),
		strings.Repeat("日本語テキスト ", 20),
		strings.Repeat("📚 books 🌳 tree ", 15),
		"⚠️⚠️⚠️⚠️⚠️ " + strings.Repeat("x", 100),
		strings.Repeat("— em dash · middot ", 20),
		"",
		"short",
	}
	for _, w := range []int{20, 40, 80, 120} {
		for _, in := range inputs {
			got := truncateToWidth(in, w, "...")
			if lw := lipgloss.Width(got); lw > w {
				t.Errorf("truncateToWidth(%.20q…, %d) → %d cells by lipgloss, over budget",
					in, w, lw)
			}
		}
	}
}

// ⚠️ AND THE WHOLE PREVIEW PANE MUST FIT ITS PANEL. This is the assertion that
// would have caught the original bug: render the pane at a width and check
// every line against the ruler the join uses.
func TestPreviewPaneLinesFitTheirPanel(t *testing.T) {
	for _, w := range []int{40, 60, 100, 140} {
		home := NewHome()
		home.width, home.height = w*2, 40
		home.updateSizes()
		out := home.renderPreviewPane(w, 30)
		for i, line := range strings.Split(out, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("preview at width %d: line %d is %d cells", w, i, lw)
				break
			}
		}
	}
}

// The helper must agree with lipgloss by construction — it is lipgloss.
func TestDisplayWidthIsTheJoinsRuler(t *testing.T) {
	for _, s := range []string{"⚠️", "📚", "日本語", "plain", "", "—·"} {
		if displayWidth(s) != lipgloss.Width(s) {
			t.Errorf("displayWidth(%q) = %d, lipgloss.Width = %d", s, displayWidth(s), lipgloss.Width(s))
		}
	}
}
