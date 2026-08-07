package session

import "testing"

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

// TestColorSchemesAllDark guards Eric's requirement: every preset is dark-friendly
// (real background set, never the light/inherit case).
func TestColorSchemesAllDark(t *testing.T) {
	if ColorSchemes[0].Name != DefaultColorSchemeName {
		t.Errorf("ColorSchemes[0] = %q, must equal DefaultColorSchemeName %q",
			ColorSchemes[0].Name, DefaultColorSchemeName)
	}
	for _, cs := range ColorSchemes {
		if cs.Bg == "" || cs.Fg == "" || cs.Accent == "" {
			t.Errorf("scheme %q must define Bg/Fg/Accent (got bg=%q fg=%q accent=%q)",
				cs.Name, cs.Bg, cs.Fg, cs.Accent)
		}
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
