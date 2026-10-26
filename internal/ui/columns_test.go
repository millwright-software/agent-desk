package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/millwright-software/agent-desk/internal/session"
)

// The right-edge columns are the same width on every row, whatever the
// model, the auto bolt, the turn count or a snooze, so they line up.
func TestSessionColumnsFixedWidth(t *testing.T) {
	h := &Home{}
	want := colModelW + 1 + colTurnsW + 1 + colAgeW
	mk := func(tool, model, opts string) *session.Instance {
		i := &session.Instance{ID: model, Title: model, Tool: tool, ClaudeModel: model}
		if opts != "" {
			i.ToolOptionsJSON = []byte(opts)
		}
		return i
	}
	rows := []*session.Instance{
		mk("claude", "claude-opus-5-5", ""),
		mk("claude", "claude-fable-5-1", `{"tool":"claude","options":{"skip_permissions":true}}`),
		mk("shell", "", ""),
		mk("claude", "some-very-long-unknown-model-id", ""),
	}
	snoozed := mk("claude", "claude-opus-5-5", "")
	snoozed.Snooze(time.Now().Add(72 * time.Hour))
	rows = append(rows, snoozed)
	for _, r := range rows {
		for _, sel := range []bool{false, true} {
			got := h.sessionColumns(r, sel)
			if w := lipgloss.Width(got); w != want {
				t.Errorf("%q selected=%v: width %d, want %d: %q", r.ID, sel, w, want, got)
			}
		}
	}
}

// The model column is the family only: no version.
func TestModelFamily(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5-5":            "o",
		"claude-fable-5-1":           "f",
		"claude-sonnet-5-5":          "s",
		"claude-haiku-4-5-20251001":  "h",
		"claude-3-5-sonnet-20241022": "s",
		"":                           "",
	}
	for id, want := range cases {
		i := &session.Instance{Tool: "claude", ClaudeModel: id}
		if got := i.ModelFamily(); got != want {
			t.Errorf("%q: got %q want %q", id, got, want)
		}
	}
	g := &session.Instance{Tool: "gemini", GeminiModel: "gemini-2.5-pro"}
	if got := g.ModelFamily(); got != "g" {
		t.Errorf("gemini: got %q", got)
	}
}

// Rows render to exactly the list width, with the title truncated to make room.
func TestSessionRowFitsWidth(t *testing.T) {
	h := &Home{listRowWidth: 40}
	long := &session.Instance{ID: "x", Title: strings.Repeat("Long title ", 8), Tool: "claude", ClaudeModel: "claude-opus-5-5"}
	short := &session.Instance{ID: "y", Title: "Phone", Tool: "claude", ClaudeModel: "claude-opus-5-5"}
	for _, inst := range []*session.Instance{long, short} {
		var b strings.Builder
		h.renderSessionItem(&b, session.Item{Type: session.ItemTypeSession, Session: inst, Level: 1}, false)
		line := strings.TrimSuffix(b.String(), "\n")
		if w := lipgloss.Width(line); w != 39 {
			t.Errorf("%s: row width %d, want 39: %q", inst.ID, w, line)
		}
	}
}

func TestColumnAge(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	cases := map[time.Duration]string{
		10 * time.Second:    "1m",
		40 * time.Minute:    "40m",
		59 * time.Minute:    "59m",
		5 * time.Hour:       "5h",
		23 * time.Hour:      "23h",
		3 * 24 * time.Hour:  "3d",
		6 * 24 * time.Hour:  "6d",
		7 * 24 * time.Hour:  "1w",
		20 * 24 * time.Hour: "2w",
		21 * 24 * time.Hour: "3w+",
		90 * 24 * time.Hour: "3w+",
	}
	for d, want := range cases {
		if got := columnAge(now.Add(-d), now); got != want {
			t.Errorf("%v: got %q want %q", d, got, want)
		}
		if lipgloss.Width(columnAge(now.Add(-d), now)) > colAgeW {
			t.Errorf("%v overflows the age column", d)
		}
	}
}

// Columns shrink to the widest value in the list: no auto session and no
// turns leaves "o · 5d" (model 1, turns 1, age 2), not the 11-cell cap.
func TestFitColumnsShrinksToList(t *testing.T) {
	now := time.Now()
	a := &session.Instance{ID: "a", Tool: "claude", ClaudeModel: "claude-opus-5-5"}
	a.CreatedAt = now.Add(-5 * 24 * time.Hour)
	items := []session.Item{{Type: session.ItemTypeSession, Session: a}}
	got := fitColumns(items, now)
	if want := (colWidths{model: 1, turns: 1, age: 2}); got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	h := &Home{colW: got}
	if w := lipgloss.Width(h.sessionColumns(a, false)); w != got.total() {
		t.Errorf("row width %d, want %d", w, got.total())
	}

	// A snoozed row's wake time widens the block so it still fits.
	s := &session.Instance{ID: "s", Tool: "claude", ClaudeModel: "claude-opus-5-5"}
	s.Snooze(now.Add(72 * time.Hour))
	items = append(items, session.Item{Type: session.ItemTypeSession, Session: s})
	got = fitColumns(items, now)
	wake := session.FormatSnoozeUntil(s.SnoozeUntil, now)
	if got.total() < lipgloss.Width(wake) {
		t.Errorf("total %d narrower than wake %q", got.total(), wake)
	}
	h.colW = got
	for _, inst := range []*session.Instance{a, s} {
		if w := lipgloss.Width(h.sessionColumns(inst, false)); w != got.total() {
			t.Errorf("%s: width %d, want %d", inst.ID, w, got.total())
		}
	}
}
