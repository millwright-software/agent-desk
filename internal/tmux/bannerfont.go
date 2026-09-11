//go:build !windows
// +build !windows

package tmux

import "strings"

// bannerFontRows is the height of the block font used on the landing card.
const bannerFontRows = 5

// bannerFont is a 5-row block font. Lowercase is drawn as uppercase; anything
// not here is drawn as itself on the middle row, so an unexpected character
// degrades to legible rather than to a blank.
var bannerFont = map[rune][bannerFontRows]string{
	'A': {" ██ ", "█  █", "████", "█  █", "█  █"},
	'B': {"███ ", "█  █", "███ ", "█  █", "███ "},
	'C': {" ███", "█   ", "█   ", "█   ", " ███"},
	'D': {"███ ", "█  █", "█  █", "█  █", "███ "},
	'E': {"████", "█   ", "███ ", "█   ", "████"},
	'F': {"████", "█   ", "███ ", "█   ", "█   "},
	'G': {" ███", "█   ", "█ ██", "█  █", " ███"},
	'H': {"█  █", "█  █", "████", "█  █", "█  █"},
	'I': {"███", " █ ", " █ ", " █ ", "███"},
	'J': {"  ██", "   █", "   █", "█  █", " ██ "},
	'K': {"█  █", "█ █ ", "██  ", "█ █ ", "█  █"},
	'L': {"█   ", "█   ", "█   ", "█   ", "████"},
	'M': {"█   █", "██ ██", "█ █ █", "█   █", "█   █"},
	'N': {"█   █", "██  █", "█ █ █", "█  ██", "█   █"},
	'O': {" ██ ", "█  █", "█  █", "█  █", " ██ "},
	'P': {"███ ", "█  █", "███ ", "█   ", "█   "},
	'Q': {" ██ ", "█  █", "█  █", "█ ██", " ███"},
	'R': {"███ ", "█  █", "███ ", "█ █ ", "█  █"},
	'S': {" ███", "█   ", " ██ ", "   █", "███ "},
	'T': {"███", " █ ", " █ ", " █ ", " █ "},
	'U': {"█  █", "█  █", "█  █", "█  █", " ██ "},
	'V': {"█   █", "█   █", "█   █", " █ █ ", "  █  "},
	'W': {"█   █", "█   █", "█ █ █", "██ ██", "█   █"},
	'X': {"█  █", "█  █", " ██ ", "█  █", "█  █"},
	'Y': {"█   █", " █ █ ", "  █  ", "  █  ", "  █  "},
	'Z': {"████", "   █", " ██ ", "█   ", "████"},
	'0': {" ██ ", "█  █", "█  █", "█  █", " ██ "},
	'1': {" █ ", "██ ", " █ ", " █ ", "███"},
	'2': {"███ ", "   █", " ██ ", "█   ", "████"},
	'3': {"███ ", "   █", " ██ ", "   █", "███ "},
	'4': {"█  █", "█  █", "████", "   █", "   █"},
	'5': {"████", "█   ", "███ ", "   █", "███ "},
	'6': {" ██ ", "█   ", "███ ", "█  █", " ██ "},
	'7': {"████", "   █", "  █ ", " █  ", " █  "},
	'8': {" ██ ", "█  █", " ██ ", "█  █", " ██ "},
	'9': {" ██ ", "█  █", " ███", "   █", " ██ "},
	' ': {"  ", "  ", "  ", "  ", "  "},
	'-': {"   ", "   ", "███", "   ", "   "},
	'_': {"   ", "   ", "   ", "   ", "███"},
	'.': {" ", " ", " ", " ", "█"},
	':': {" ", "█", " ", "█", " "},
	'/': {"   █", "  █ ", "  █ ", " █  ", "█   "},
}

// bannerGlyph returns the rows for one character, all the same width.
func bannerGlyph(r rune) [bannerFontRows]string {
	if 'a' <= r && r <= 'z' {
		r -= 'a' - 'A'
	}
	if g, ok := bannerFont[r]; ok {
		return g
	}
	// Not in the font: the character itself, mid-row, one cell wide.
	return [bannerFontRows]string{" ", " ", string(r), " ", " "}
}

// renderBlockText draws s in the block font as bannerFontRows lines, glyphs
// separated by one blank column. Rows are padded to equal width so centring
// the lines individually keeps them aligned.
func renderBlockText(s string) []string {
	rows := make([]string, bannerFontRows)
	for i, r := range s {
		g := bannerGlyph(r)
		for row := range rows {
			if i > 0 {
				rows[row] += " "
			}
			rows[row] += g[row]
		}
	}
	return rows
}

// blockTextWidth is the width in cells of renderBlockText(s).
func blockTextWidth(s string) int {
	rows := renderBlockText(s)
	if len(rows) == 0 {
		return 0
	}
	return len([]rune(rows[0]))
}

// blockTextFits reports whether the block rendering of s fits in cols.
func blockTextFits(s string, cols int) bool {
	return strings.TrimSpace(s) != "" && blockTextWidth(s) <= cols
}
