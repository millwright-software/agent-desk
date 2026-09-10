package ui

import (
	"testing"

	"github.com/millwright-software/agent-desk/internal/session"
)

// Shift+Right / Shift+Left inside an attach walk the sessions shown in the
// sidebar, skipping dead ones and group headers, and crossing group boundaries.

func inst(id string) *session.Instance {
	return &session.Instance{ID: id, Title: id, Tool: "claude"}
}

// A board with two groups, a dead session in the middle of the first, and a
// group header between the groups — the shape the ring has to survive.
func fixture() ([]session.Item, map[string]*session.Instance) {
	api := inst("api-refactor")
	dead := inst("old-spike")
	desk := inst("agent-desk")
	notes := inst("notes")
	items := []session.Item{
		{Type: session.ItemTypeGroup, Path: "Work"},
		{Type: session.ItemTypeSession, Session: api},
		{Type: session.ItemTypeSession, Session: dead},
		{Type: session.ItemTypeGroup, Path: "Personal"},
		{Type: session.ItemTypeSession, Session: desk},
		{Type: session.ItemTypeSession, Session: notes},
	}
	return items, map[string]*session.Instance{
		"api": api, "dead": dead, "desk": desk, "notes": notes,
	}
}

// live reports everything alive except the ids named.
func live(deadIDs ...string) func(*session.Instance) bool {
	dead := map[string]bool{}
	for _, id := range deadIDs {
		dead[id] = true
	}
	return func(i *session.Instance) bool { return !dead[i.ID] }
}

func TestRingSkipsDeadSessionsAndGroupHeaders(t *testing.T) {
	items, s := fixture()
	isLive := live("old-spike")

	// api-refactor → agent-desk: steps over the dead session AND the "Personal"
	// group header. A ring that stopped at group boundaries would return nil.
	if got := neighbourInRing(items, s["api"], true, isLive); got != s["desk"] {
		t.Errorf("next from api-refactor = %v, want agent-desk", idOf(got))
	}
	if got := neighbourInRing(items, s["desk"], true, isLive); got != s["notes"] {
		t.Errorf("next from agent-desk = %v, want notes", idOf(got))
	}
}

func TestRingWrapsBothWays(t *testing.T) {
	items, s := fixture()
	isLive := live("old-spike")

	if got := neighbourInRing(items, s["notes"], true, isLive); got != s["api"] {
		t.Errorf("next from the last session = %v, want api-refactor (wrap)", idOf(got))
	}
	if got := neighbourInRing(items, s["api"], false, isLive); got != s["notes"] {
		t.Errorf("prev from the first session = %v, want notes (wrap)", idOf(got))
	}
}

func TestRingPrevIsTheInverseOfNext(t *testing.T) {
	items, s := fixture()
	isLive := live("old-spike")
	for _, start := range []*session.Instance{s["api"], s["desk"], s["notes"]} {
		next := neighbourInRing(items, start, true, isLive)
		if back := neighbourInRing(items, next, false, isLive); back != start {
			t.Errorf("next then prev from %s landed on %v, want %s",
				start.ID, idOf(back), start.ID)
		}
	}
}

// ⚠️ The key must do NOTHING rather than re-attach to the session you are
// already in — a silent re-attach looks exactly like a broken keybinding.
func TestRingReturnsNilWhenNothingElseIsLive(t *testing.T) {
	items, s := fixture()
	isLive := live("old-spike", "agent-desk", "notes")
	if got := neighbourInRing(items, s["api"], true, isLive); got != nil {
		t.Errorf("only one live session: next = %v, want nil", idOf(got))
	}
	if got := neighbourInRing(items, s["api"], false, isLive); got != nil {
		t.Errorf("only one live session: prev = %v, want nil", idOf(got))
	}
}

func TestRingIsEmptyWhenNoSessionIsLive(t *testing.T) {
	items, s := fixture()
	none := func(*session.Instance) bool { return false }
	if got := neighbourInRing(items, s["api"], true, none); got != nil {
		t.Errorf("no live sessions: next = %v, want nil", idOf(got))
	}
}

// The session we were attached to can die while we are in it — its command
// exits, tmux tears the session down, and then Shift+Right asks for the
// neighbour of something no longer in the ring. Landing somewhere live beats
// doing nothing.
func TestRingHandlesTheSourceSessionDying(t *testing.T) {
	items, s := fixture()
	isLive := live("old-spike", "api-refactor") // we were in api-refactor; it died
	got := neighbourInRing(items, s["api"], true, isLive)
	if got != s["desk"] {
		t.Errorf("source session dead: next = %v, want agent-desk (first live)", idOf(got))
	}
}

// A group header carrying a nil Session must never be dereferenced.
func TestRingIgnoresNilSessionsOnItems(t *testing.T) {
	a, b := inst("a"), inst("b")
	items := []session.Item{
		{Type: session.ItemTypeSession, Session: a},
		{Type: session.ItemTypeSession, Session: nil},
		{Type: session.ItemTypeGroup, Path: "G"},
		{Type: session.ItemTypeSession, Session: b},
	}
	if got := neighbourInRing(items, a, true, live()); got != b {
		t.Errorf("next = %v, want b", idOf(got))
	}
}

func idOf(i *session.Instance) string {
	if i == nil {
		return "<nil>"
	}
	return i.ID
}
