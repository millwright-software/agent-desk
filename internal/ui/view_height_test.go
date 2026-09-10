package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"testing"

	"github.com/millwright-software/agent-desk/internal/session"
)

// ⚠️ VIEW() MUST NEVER EMIT MORE LINES THAN THE TERMINAL HAS. One line too many
// and the terminal scrolls: the top of the frame — the first group's header and
// the "⋮ +N above" indicator — slides off, and because Bubble Tea then loses
// cursor tracking, every later frame stacks on the last instead of replacing it.
// That is the "flickers and scrolls up on every arrow key" report, and the
// three-stacked-help-bars one before it.
//
// The height math lives in three places that must agree — View(),
// syncViewport(), and the per-layout render functions — so this asserts the
// OUTPUT rather than any one of them.
// The reporter's actual board: three groups holding 4, 8 and 5 sessions.
// A synthetic every-fifth-item-is-a-header fixture did NOT reproduce the fault;
// the real group sizes are uneven, which is what makes the arithmetic bite.
func realBoard() []session.Item {
	var items []session.Item
	for gi, n := range []int{4, 8, 5} {
		items = append(items, session.Item{
			Type: session.ItemTypeGroup, Group: &session.Group{Name: "g" + itoa(gi)}, Path: "g" + itoa(gi),
		})
		for i := 0; i < n; i++ {
			items = append(items, session.Item{
				Type: session.ItemTypeSession, Level: 1,
				Session: &session.Instance{ID: itoa(gi*100 + i), Title: "📚 s" + itoa(i), Tool: "claude"},
			})
		}
	}
	return items
}

func TestRealBoardViewportIsStable(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}, {120, 25}, {200, 50}, {90, 20}} {
		home := NewHome()
		home.width, home.height = sz.w, sz.h
		home.flatItems = realBoard()
		home.updateSizes()

		// ⚠️ ARROWING DOWN THE LIST MUST NEVER SCROLL THE VIEW BACKWARDS. The
		// offset may hold or advance; if it ever decreases while the cursor is
		// moving forward, the list visibly jumps upward under the user's hand.
		home.cursor, home.viewOffset = 0, 0
		home.syncViewport()
		prev := home.viewOffset
		for c := 1; c < len(home.flatItems); c++ {
			home.cursor = c
			home.syncViewport()
			if home.viewOffset < prev {
				t.Errorf("%dx%d: moving cursor %d→%d scrolled BACK, offset %d→%d",
					sz.w, sz.h, c-1, c, prev, home.viewOffset)
				break
			}
			// ⚠️ And the cursor must stay inside what is actually drawn.
			if home.cursor < home.viewOffset {
				t.Errorf("%dx%d: cursor %d is above viewOffset %d", sz.w, sz.h, home.cursor, home.viewOffset)
				break
			}
			prev = home.viewOffset
		}

		// ⚠️ SYNC MUST BE IDEMPOTENT. It computes the new offset from a value
		// that itself depends on the old offset, so a second call with an
		// unchanged cursor must not move anything.
		before := home.viewOffset
		home.syncViewport()
		if home.viewOffset != before {
			t.Errorf("%dx%d: syncViewport is not idempotent — offset moved %d→%d with the cursor unchanged",
				sz.w, sz.h, before, home.viewOffset)
		}
	}
}

func TestViewNeverExceedsTerminalHeight(t *testing.T) {
	// Sizes worth covering: a short window, a normal one, and the wide/short
	// shape where the help bar stops wrapping and the arithmetic shifts.
	sizes := []struct{ w, h int }{
		{80, 24}, {100, 30}, {200, 50}, {120, 20}, {250, 45}, {80, 12}, {60, 10},
	}
	// Enough items to force scrolling at every one of those heights.
	counts := []int{0, 3, 12, 40}

	for _, sz := range sizes {
		for _, n := range counts {
			home := NewHome()
			home.width = sz.w
			home.height = sz.h
			home.flatItems = makeItems(n)
			home.updateSizes()
			home.syncViewport()

			// Walk the cursor through the whole list: the offset changes as it
			// goes, and the overflow only appears at certain positions.
			for cursor := 0; cursor < maxInt(n, 1); cursor++ {
				home.cursor = cursor
				home.syncViewport()
				got := strings.Count(home.View(), "\n") + 1
				if got > sz.h {
					t.Errorf("%dx%d, %d items, cursor %d: View() returned %d lines, terminal has %d (%d too many)",
						sz.w, sz.h, n, cursor, got, sz.h, got-sz.h)
					break // one report per size/count is enough
				}
			}
		}
	}
}

// ⚠️ AND IT MUST FILL THE TERMINAL EXACTLY. Too few lines is not merely untidy:
// the renderer's idea of where the frame ends drifts from the terminal's, which
// is the same class of failure from the other direction.
func TestViewFillsTerminalHeightExactly(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {200, 50}, {120, 20}} {
		home := NewHome()
		home.width = sz.w
		home.height = sz.h
		home.flatItems = makeItems(25)
		home.updateSizes()
		home.syncViewport()
		got := strings.Count(home.View(), "\n") + 1
		if got != sz.h {
			t.Errorf("%dx%d: View() returned %d lines, want exactly %d", sz.w, sz.h, got, sz.h)
		}
	}
}

// ⚠️ NO LINE MAY BE WIDER THAN THE TERMINAL EITHER. A line one cell too wide
// wraps, which adds a line, which overflows the height — the same crash landing
// by a different road.
func TestViewNeverExceedsTerminalWidth(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}, {200, 50}, {60, 20}} {
		home := NewHome()
		home.width = sz.w
		home.height = sz.h
		home.flatItems = makeItems(20)
		home.updateSizes()
		home.syncViewport()
		for i, line := range strings.Split(home.View(), "\n") {
			if w := lipgloss.Width(line); w > sz.w {
				t.Errorf("%dx%d: line %d is %d cells wide, terminal is %d", sz.w, sz.h, i, w, sz.w)
				break
			}
		}
	}
}

// A group header followed by sessions, repeated — the shape of a real sidebar,
// including the emoji titles that make width measurement interesting.
func makeItems(n int) []session.Item {
	items := make([]session.Item, 0, n)
	for i := 0; i < n; i++ {
		if i%5 == 0 {
			items = append(items, session.Item{
				Type:  session.ItemTypeGroup,
				Group: &session.Group{Name: "group-" + itoa(i)},
				Path:  "group-" + itoa(i),
			})
			continue
		}
		items = append(items, session.Item{
			Type:  session.ItemTypeSession,
			Level: 1,
			Session: &session.Instance{
				ID:    "id-" + itoa(i),
				Title: "📚 session-" + itoa(i),
				Tool:  "claude",
			},
		})
	}
	return items
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
