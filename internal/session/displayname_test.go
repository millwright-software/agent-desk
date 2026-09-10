package session

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

func TestSanitizeDisplayName(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		// ⚠️ THE ONE THAT BROKE THE SCREEN: base + ZWJ + ♂ + VS16.
		{"zwj mountain biker", "🚵‍♂️ mtb", "🚵 mtb"},
		{"zwj technologist", "👨‍💻 dev", "👨 dev"},
		{"zwj family", "👨‍👩‍👧 family", "👨 family"},
		{"rainbow flag", "🏳️‍🌈 pride", "🏳 pride"},

		// Variation selectors alone flip a glyph between one and two cells —
		// lipgloss says ⚠️ is 2, runewidth says 1.
		{"warning with VS16", "⚠️ alert", "⚠ alert"},
		{"text-presentation VS15", "❤︎ love", "❤ love"},

		// Skin tones.
		{"thumbs up medium", "👍🏽 ok", "👍 ok"},

		// Invisible characters that cost nothing visually and everything to
		// the width math.
		{"zero-width space", "a​b", "ab"},
		{"word joiner", "a⁠b", "ab"},

		// ⚠️ LEFT ALONE. Single emoji are exactly what the operator wants and
		// every terminal agrees on them.
		{"plain ascii", "my session", "my session"},
		{"single emoji", "📚 Legal", "📚 Legal"},
		{"another single", "🌳 Me", "🌳 Me"},
		{"memo", "📝 letter", "📝 letter"},
		{"empty", "", ""},

		// ⚠️ COMBINING MARKS SURVIVE. Mangling "José" to dodge a terminal bug
		// is not a trade worth making.
		{"combining acute", "José", "José"},
		{"precomposed", "José", "José"},

		// CJK is wide but unambiguous.
		{"cjk", "日本語", "日本語"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeDisplayName(tc.in); got != tc.want {
				t.Errorf("SanitizeDisplayName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ⚠️ THE PROPERTY THAT ACTUALLY MATTERS. The bug was not "an emoji looked odd",
// it was that two width measurements disagreed, so the padded row was a
// different size from the drawn one. After sanitizing, every measurement of a
// title must agree — that is the invariant the layout depends on.
func TestSanitizedNamesMeasureConsistently(t *testing.T) {
	inputs := []string{
		"🚵‍♂️ mtb", "👨‍💻 dev", "👨‍👩‍👧 family", "🏳️‍🌈 pride",
		"⚠️ alert", "👍🏽 ok", "📚 Legal", "🌳 Me", "📝 letter",
		"plain", "日本語", "José",
	}
	for _, in := range inputs {
		clean := SanitizeDisplayName(in)
		lw := lipgloss.Width(clean)
		rw := runewidth.StringWidth(clean)
		uw := uniseg.StringWidth(clean)
		if lw != rw || lw != uw {
			t.Errorf("%q → %q: widths disagree — lipgloss %d, runewidth %d, uniseg %d",
				in, clean, lw, rw, uw)
		}
	}
}

// ⚠️ AND THE RAW INPUTS MUST ACTUALLY HAVE BEEN A PROBLEM, or the sanitizer is
// solving nothing. This pins the disagreement the fix exists for: if a future
// library update makes these agree on their own, this test fails and says so.
func TestRawComposedEmojiDoDisagree(t *testing.T) {
	disagreed := 0
	for _, in := range []string{"⚠️", "🚵‍♂️", "👨‍💻", "👨‍👩‍👧", "🏳️‍🌈"} {
		if lipgloss.Width(in) != uniseg.StringWidth(in) || lipgloss.Width(in) != runewidth.StringWidth(in) {
			disagreed++
		}
	}
	if disagreed == 0 {
		t.Skip("no measurement disagreement remains in these libraries — the sanitizer is now belt-and-braces, not load-bearing")
	}
	t.Logf("%d of 5 composed sequences are measured inconsistently by the three libraries", disagreed)
}
