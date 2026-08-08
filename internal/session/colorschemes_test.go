package session

import (
	"strconv"
	"testing"
)

// isLightHex reports whether a "#rrggbb" color is light (perceived luminance
// above the midpoint). Used to assert light presets really are light.
func isLightHex(hex string) bool {
	if len(hex) != 7 || hex[0] != '#' {
		return false
	}
	v := func(s string) float64 {
		n, _ := strconv.ParseInt(s, 16, 0)
		return float64(n)
	}
	r, g, b := v(hex[1:3]), v(hex[3:5]), v(hex[5:7])
	// Rec. 601 luma.
	return (0.299*r + 0.587*g + 0.114*b) > 140
}

func TestColorSchemeByName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"", "Ember"},               // empty -> default (Ember)
		{"Fern", "Fern"},            // exact
		{"fern", "Fern"},            // case-insensitive
		{"GARNET", "Garnet"},        // case-insensitive upper
		{"does-not-exist", "Ember"}, // unknown -> default fallback
	}
	for _, tt := range tests {
		if got := ColorSchemeByName(tt.name).Name; got != tt.want {
			t.Errorf("ColorSchemeByName(%q).Name = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestColorSchemeStyles(t *testing.T) {
	// The default scheme (Ember) is a real dark theme, not "inherit terminal".
	def := ColorSchemeByName("")
	if !def.IsDefault() {
		t.Error("empty name should resolve to the default scheme")
	}
	if def.Name != DefaultColorSchemeName {
		t.Errorf("default Name = %q, want %q", def.Name, DefaultColorSchemeName)
	}
	if got, want := def.WindowStyle(), "bg=#0b0e14,fg=#bfbdb6"; got != want {
		t.Errorf("Ember WindowStyle() = %q, want %q", got, want)
	}
	if got, want := def.StatusStyle(), "bg=#0b0e14,fg=#ffb454"; got != want {
		t.Errorf("Ember StatusStyle() = %q, want %q", got, want)
	}

	c := ColorSchemeByName("Cobalt")
	if c.IsDefault() {
		t.Error("Cobalt should not be the default scheme")
	}
	if got, want := c.WindowStyle(), "bg=#182a4f,fg=#cdd9f5"; got != want {
		t.Errorf("Cobalt WindowStyle() = %q, want %q", got, want)
	}
	if got, want := c.StatusStyle(), "bg=#182a4f,fg=#5b9cff"; got != want {
		t.Errorf("Cobalt StatusStyle() = %q, want %q", got, want)
	}
}

// TestColorSchemesWellFormed guards that every preset defines Bg/Fg/Accent, the
// first scheme is the (dark) default, and at least one light preset exists for
// Copilot-style tools — including the one CopilotDefaultColorScheme points at.
func TestColorSchemesWellFormed(t *testing.T) {
	if ColorSchemes[0].Name != DefaultColorSchemeName {
		t.Errorf("ColorSchemes[0] = %q, must equal DefaultColorSchemeName %q",
			ColorSchemes[0].Name, DefaultColorSchemeName)
	}
	lightCount := 0
	for _, cs := range ColorSchemes {
		if cs.Bg == "" || cs.Fg == "" || cs.Accent == "" {
			t.Errorf("scheme %q must define Bg/Fg/Accent (got bg=%q fg=%q accent=%q)",
				cs.Name, cs.Bg, cs.Fg, cs.Accent)
		}
		if isLightHex(cs.Bg) {
			lightCount++
		}
	}
	if lightCount == 0 {
		t.Error("expected at least one light preset for Copilot-style tools")
	}
	// The default is dark; the Copilot default is one of the light presets and
	// must resolve to a real, non-fallback scheme.
	if isLightHex(ColorSchemeByName("").Bg) {
		t.Error("default scheme must be dark")
	}
	if got := ColorSchemeByName(CopilotDefaultColorScheme); got.Name != CopilotDefaultColorScheme {
		t.Errorf("CopilotDefaultColorScheme %q does not resolve to a real scheme (got %q)",
			CopilotDefaultColorScheme, got.Name)
	}
}

func TestSetColorScheme(t *testing.T) {
	inst := NewInstance("test", "/tmp")

	// Applying a non-default scheme stores the canonical name.
	inst.SetColorScheme("garnet")
	if inst.ColorScheme != "Garnet" {
		t.Errorf("after SetColorScheme(\"garnet\"), ColorScheme = %q, want \"Garnet\"", inst.ColorScheme)
	}

	// Applying the default scheme clears the stored value (stays omitempty).
	inst.SetColorScheme("Ember")
	if inst.ColorScheme != "" {
		t.Errorf("after SetColorScheme(\"Ember\"), ColorScheme = %q, want \"\"", inst.ColorScheme)
	}
}
