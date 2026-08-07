package session

import (
	"fmt"

	"github.com/millwright-software/agent-desk/internal/tmux"
)

// ColorScheme defines a per-session visual theme. The same scheme is applied to
// the attached tmux window (pane background + status bar) and to the agent-desk
// preview pane (title swatch + accent), so a session is recognizable on both
// surfaces.
//
// Bg/Fg are the pane colors. Accent is used for the tmux status bar foreground
// and the preview/sidebar accent in the TUI. All presets are dark-friendly.
// (Empty Bg+Fg is reserved to mean "inherit the terminal's own colors", but no
// preset currently uses it.)
type ColorScheme struct {
	Name   string // display name and stored key (case-insensitive match)
	Bg     string // hex pane background, e.g. "#1a1b26"
	Fg     string // hex pane foreground
	Accent string // hex accent for status bar + preview/sidebar accent
}

// DefaultColorSchemeName is the scheme applied to sessions that have none set
// explicitly. It must match ColorSchemes[0].Name.
const DefaultColorSchemeName = "Ember"

// ColorSchemes is the ordered preset palette shown in the picker, and the single
// source of truth for both tmux and TUI styling. ColorSchemes[0] is the default.
//
// Each background is a saturated-but-dark tint so its hue is obvious at a glance
// (in the spirit of classic Gruvbox brown / Solarized teal), and each name
// evokes the color the pane glows. Backgrounds walk the color wheel
// (warm -> cool) with one neutral; all are dark-friendly (dark bg, light fg).
var ColorSchemes = []ColorScheme{
	{Name: "Ember", Bg: "#0b0e14", Fg: "#bfbdb6", Accent: "#ffb454"},    // dark coal + orange glow (default)
	{Name: "Garnet", Bg: "#3d1f23", Fg: "#f2d4d6", Accent: "#ff5f6b"},   // red
	{Name: "Brass", Bg: "#333317", Fg: "#e8e6c0", Accent: "#c8d44f"},    // olive / chartreuse
	{Name: "Fern", Bg: "#1d3a26", Fg: "#cfe8d2", Accent: "#5fd07a"},     // green
	{Name: "Cobalt", Bg: "#182a4f", Fg: "#cdd9f5", Accent: "#5b9cff"},   // blue
	{Name: "Cosmos", Bg: "#242350", Fg: "#d8d4f2", Accent: "#8a7bf0"},   // indigo
	{Name: "Mulberry", Bg: "#3a2038", Fg: "#f0d4ea", Accent: "#ff6fd0"}, // magenta
	{Name: "Slate", Bg: "#2b3340", Fg: "#d6dde8", Accent: "#8fb3d9"},    // neutral steel
}

// ColorSchemeByName returns the scheme with the given name (case-insensitive).
// An empty name resolves to the Default scheme. Unknown names also fall back to
// Default so a renamed/removed preset never breaks a persisted session.
func ColorSchemeByName(name string) ColorScheme {
	if name == "" {
		return ColorSchemes[0]
	}
	for _, cs := range ColorSchemes {
		if equalFoldASCII(cs.Name, name) {
			return cs
		}
	}
	return ColorSchemes[0]
}

// IsDefault reports whether this is the default scheme (the one applied to
// sessions with no explicit choice). Used to suppress the "custom color" marker
// in the TUI for default sessions.
func (cs ColorScheme) IsDefault() bool {
	return cs.Name == DefaultColorSchemeName
}

// inheritsTerminal reports whether the scheme applies no pane colors (so the
// terminal's own colors show through). No current preset does this.
func (cs ColorScheme) inheritsTerminal() bool {
	return cs.Bg == "" && cs.Fg == ""
}

// applyColorSchemeToTmux copies a scheme's tmux styling onto a session struct.
// Nil-safe; does not touch the live session (use Session.ApplyColorScheme for that).
func applyColorSchemeToTmux(sess *tmux.Session, schemeName string) {
	if sess == nil {
		return
	}
	cs := ColorSchemeByName(schemeName)
	sess.WindowStyle = cs.WindowStyle()
	sess.StatusStyle = cs.StatusStyle()
}

// WindowStyle returns the tmux window-style/window-active-style value for this
// scheme, e.g. "bg=#1a1b26,fg=#c0caf5". Returns "default" when no pane colors
// are set, which tells tmux to inherit the terminal's own colors.
func (cs ColorScheme) WindowStyle() string {
	if cs.inheritsTerminal() {
		return "default"
	}
	return fmt.Sprintf("bg=%s,fg=%s", cs.Bg, cs.Fg)
}

// StatusStyle returns the tmux status-style value (status bar bg/fg). Returns ""
// only for an inherit-terminal scheme, signaling callers to keep the built-in
// status bar styling.
func (cs ColorScheme) StatusStyle() string {
	if cs.inheritsTerminal() {
		return ""
	}
	return fmt.Sprintf("bg=%s,fg=%s", cs.Bg, cs.Accent)
}

// equalFoldASCII is a tiny case-insensitive compare that avoids pulling in
// strings just for ColorSchemeByName.
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
