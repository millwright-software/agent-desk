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
		t.Errorf("source session dead: next = %v, want agent-desk (next below)", idOf(got))
	}
	// Backwards from the dead one: nothing above it, so wrap to the bottom.
	if got := neighbourInRing(items, s["api"], false, isLive); got != s["notes"] {
		t.Errorf("source session dead: prev = %v, want notes (wrap to bottom)", idOf(got))
	}
}

// Shift+Down / Shift+Up: the predicate is "waiting on you", and the session
// being left is normally NOT a candidate (attaching acknowledged it). The walk
// must then go by list position: down finds the next waiting one below, up
// the next above, each wrapping at the ends.
func TestWaitingRingWalksFromListPosition(t *testing.T) {
	items, s := fixture() // list order: api, old-spike, desk, notes
	waiting := func(ids ...string) func(*session.Instance) bool {
		w := map[string]bool{}
		for _, id := range ids {
			w[id] = true
		}
		return func(i *session.Instance) bool { return w[i.ID] }
	}

	// From old-spike (not waiting): down → desk, up → api.
	isWaiting := waiting("api-refactor", "agent-desk")
	if got := neighbourInRing(items, s["dead"], true, isWaiting); got != s["desk"] {
		t.Errorf("down from old-spike = %v, want agent-desk", idOf(got))
	}
	if got := neighbourInRing(items, s["dead"], false, isWaiting); got != s["api"] {
		t.Errorf("up from old-spike = %v, want api-refactor", idOf(got))
	}

	// From notes (bottom, not waiting): down wraps to the top waiting one.
	if got := neighbourInRing(items, s["notes"], true, isWaiting); got != s["api"] {
		t.Errorf("down from notes = %v, want api-refactor (wrap)", idOf(got))
	}
	// From api (top, not waiting): up wraps to the bottom waiting one.
	isWaiting = waiting("agent-desk", "notes")
	if got := neighbourInRing(items, s["api"], false, isWaiting); got != s["notes"] {
		t.Errorf("up from api = %v, want notes (wrap)", idOf(got))
	}

	// Only one session waiting and we are not it: that is still a move.
	isWaiting = waiting("notes")
	if got := neighbourInRing(items, s["api"], true, isWaiting); got != s["notes"] {
		t.Errorf("single waiting session should be reachable, got %v", idOf(got))
	}
	// Nothing waiting: nil, and the caller says so.
	if got := neighbourInRing(items, s["api"], true, waiting()); got != nil {
		t.Errorf("nothing waiting should be nil, got %v", idOf(got))
	}
	// Parked red is skipped here too.
	s["notes"].Flag = session.FlagParkedRed
	if got := neighbourInRing(items, s["api"], true, waiting("notes")); got != nil {
		t.Errorf("a parked-red waiting session is still skipped, got %v", idOf(got))
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

// A session parked red with the u key is "shelved, skip this": the ring
// passes over it even though its tmux session is alive. Blue parking and the
// unread bookmark are not skips.
func TestRingSkipsParkedRedSessions(t *testing.T) {
	items, s := fixture()
	isLive := live() // everything alive, including old-spike this time
	s["dead"].Flag = session.FlagParkedRed

	if got := neighbourInRing(items, s["api"], true, isLive); got != s["desk"] {
		t.Errorf("next from api should skip the parked-red session, got %v", got)
	}
	if got := neighbourInRing(items, s["desk"], false, isLive); got != s["api"] {
		t.Errorf("prev from desk should skip the parked-red session, got %v", got)
	}

	s["dead"].Flag = session.FlagParkedBlue
	if got := neighbourInRing(items, s["api"], true, isLive); got != s["dead"] {
		t.Errorf("blue parking is not a skip, got %v", got)
	}
	s["dead"].Flag = session.FlagUnread
	if got := neighbourInRing(items, s["api"], true, isLive); got != s["dead"] {
		t.Errorf("unread is not a skip, got %v", got)
	}

	// Everything else parked red: nothing to switch to, back to the sidebar.
	for _, id := range []string{"dead", "desk", "notes"} {
		s[id].Flag = session.FlagParkedRed
	}
	if got := neighbourInRing(items, s["api"], true, isLive); got != nil {
		t.Errorf("with every other session parked red, want nil, got %v", got)
	}
}
