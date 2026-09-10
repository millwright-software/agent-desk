package session

import (
	"strings"

	"github.com/rivo/uniseg"
)

// SanitizeDisplayName strips the emoji-composition characters that make a
// string's rendered width unpredictable, and returns something the layout can
// measure correctly.
//
// ⚠️ WHY THIS EXISTS: A COMPOSED EMOJI BROKE THE WHOLE SCREEN, NOT JUST ITS ROW.
// A session titled with a ZWJ sequence — 🚵‍♂️, which is BASE + ZWJ + ♂ + VS16,
// four code points that are supposed to draw as one glyph — measured 2 cells to
// both lipgloss and go-runewidth, and iTerm2 drew the components separately at
// roughly 4. The sidebar padded the row to what it believed was the exact panel
// width, the real line was wider than the terminal, it wrapped, the frame became
// one row taller than the screen, and the terminal scrolled. What the operator
// saw was the top of the UI missing — the header, the filter bar and the first
// group's heading — and the list "flickering and scrolling up" on every arrow
// key, none of which looks remotely like an emoji problem.
//
// ⚠️ MEASURING MORE CAREFULLY DOES NOT FIX THIS. lipgloss and go-runewidth
// agreed with each other and were both wrong about what the terminal would
// draw; a third opinion is still an opinion. The terminal's rendering of a ZWJ
// sequence depends on its font fallback, so the only reliable move is to not
// store text whose width is a matter of opinion.
//
// What is removed:
//   - Zero-width joiner (U+200D) and the rest of the cluster it joins, so
//     🚵‍♂️ becomes 🚵 and 👨‍💻 becomes 👨 — the base emoji, which every
//     terminal agrees is two cells.
//   - Variation selectors (U+FE00–U+FE0F): ⚠️ becomes ⚠. These alone flip a
//     glyph between one and two cells, and lipgloss and runewidth disagree
//     about ⚠️ even with each other.
//   - Skin-tone modifiers (U+1F3FB–U+1F3FF): 👍🏽 becomes 👍.
//   - Other zero-width and directional formatting characters, which are
//     invisible but not free.
//
// ⚠️ COMBINING MARKS ARE DELIBERATELY KEPT. Stripping them would turn "José"
// into "Jose" wherever the é is e + U+0301, and mangling people's names to
// dodge a terminal bug is not a trade worth making. They are well-handled by
// grapheme-aware measurement in a way emoji sequences are not.
func SanitizeDisplayName(s string) string {
	if s == "" || !needsSanitizing(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	// Walk grapheme clusters, not runes: a cluster is what the terminal tries
	// to draw as one glyph, and it is the whole cluster that has to go when it
	// contains a joiner.
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		runes := g.Runes()
		hasJoiner := false
		for _, r := range runes {
			if r == zeroWidthJoiner {
				hasJoiner = true
				break
			}
		}
		if hasJoiner {
			// Keep only the base — the first code point — and drop the rest of
			// the sequence.
			if len(runes) > 0 && !isStrippable(runes[0]) {
				b.WriteRune(runes[0])
			}
			continue
		}
		for _, r := range runes {
			if !isStrippable(r) {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

const zeroWidthJoiner = '‍'

func isStrippable(r rune) bool {
	switch {
	case r == zeroWidthJoiner:
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // skin-tone modifiers
		return true
	case r >= 0x200B && r <= 0x200F: // zero-width space/non-joiner, LTR/RTL marks
		return true
	case r >= 0x2060 && r <= 0x2064: // word joiner and invisible operators
		return true
	case r >= 0xE0100 && r <= 0xE01EF: // variation selectors supplement
		return true
	}
	return false
}

// needsSanitizing keeps the common case — a plain ASCII title, or one with a
// single ordinary emoji — free of grapheme segmentation.
func needsSanitizing(s string) bool {
	for _, r := range s {
		if isStrippable(r) {
			return true
		}
	}
	return false
}
