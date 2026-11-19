package ui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/millwright-software/agent-desk/internal/session"
)

// A snoozed session is "not now", like red parking: the attached ring passes
// over it until it wakes.
func TestRingSkipsSnoozedSessions(t *testing.T) {
	items, s := fixture()
	isLive := live()
	s["dead"].Snooze(time.Now().Add(time.Hour))

	if got := neighbourInRing(items, s["api"], true, isLive); got != s["desk"] {
		t.Errorf("next from api should skip the snoozed session, got %v", idOf(got))
	}
	s["dead"].WakeFromSnooze()
	if got := neighbourInRing(items, s["api"], true, isLive); got != s["dead"] {
		t.Errorf("a woken (unread) session is reachable again, got %v", idOf(got))
	}
}

// The tick wakes only sessions whose time has passed, flips them to unread,
// and reports whether anything changed so the caller can save once.
func TestWakeDueSnoozes(t *testing.T) {
	now := time.Now()
	early := inst("early")
	late := inst("late")
	plain := inst("plain")
	early.Snooze(now.Add(-time.Minute))
	late.Snooze(now.Add(time.Hour))
	plain.Flag = session.FlagParkedRed

	h := &Home{instances: []*session.Instance{early, late, plain}}
	if woken := h.wakeDueSnoozes(now); len(woken) != 1 || woken[0] != early {
		t.Fatalf("expected only early to wake, got %v", woken)
	}
	if early.Flag != session.FlagUnread || !early.SnoozeUntil.IsZero() {
		t.Errorf("early: flag=%v until=%v, want unread/zero", early.Flag, early.SnoozeUntil)
	}
	if late.Flag != session.FlagSnoozed || late.SnoozeUntil.IsZero() {
		t.Errorf("late: flag=%v until=%v, want still snoozed", late.Flag, late.SnoozeUntil)
	}
	if plain.Flag != session.FlagParkedRed {
		t.Errorf("plain parked-red session touched: %v", plain.Flag)
	}
	if woken := h.wakeDueSnoozes(now); len(woken) != 0 {
		t.Error("second pass with nothing due should report no change")
	}
}

// Snoozing is in place: the session keeps its group, moves into the group's
// snooze fold below the live sessions (soonest wake first), and comes back to
// its manual spot on wake. Sessions left in the legacy global Snoozed group go
// home on load. order() reads with every fold open (FlattenAll).
func TestSnoozeInPlace(t *testing.T) {
	a, b, c := inst("a"), inst("b"), inst("c")
	for i, s := range []*session.Instance{a, b, c} {
		s.GroupPath, s.Order = "Work", i
	}
	h := &Home{instances: []*session.Instance{a, b, c}}
	h.groupTree = session.NewGroupTree(h.instances)
	order := func() string {
		out := ""
		for _, it := range h.groupTree.FlattenAll() {
			if it.Type == session.ItemTypeSession {
				out += it.Session.ID
			}
		}
		return out
	}

	h.snoozeSession(a, time.Now().Add(48*time.Hour))
	h.snoozeSession(b, time.Now().Add(2*time.Hour))
	if a.GroupPath != "Work" || b.GroupPath != "Work" {
		t.Fatalf("snooze moved sessions: a=%q b=%q", a.GroupPath, b.GroupPath)
	}
	if _, ok := h.groupTree.Groups[session.SnoozedGroupPath]; ok {
		t.Error("no Snoozed group should be created")
	}
	if got := order(); got != "cba" {
		t.Errorf("order = %q, want c (live) then b, a (soonest wake first)", got)
	}
	// Sinking holds under a sort key too.
	h.groupTree.SortKey = func(i *session.Instance) int64 { return map[string]int64{"a": 3, "b": 2, "c": 1}[i.ID] }
	if got := order(); got != "cba" {
		t.Errorf("sorted order = %q, want snoozed still last", got)
	}
	h.groupTree.SortKey = nil

	// Wake: back to its manual place, unread.
	b.SnoozeUntil = time.Now().Add(-time.Second)
	for _, w := range h.wakeDueSnoozes(time.Now()) {
		h.returnFromSnooze(w)
	}
	if b.Flag != session.FlagUnread || b.GroupPath != "Work" {
		t.Errorf("after wake: flag=%v group=%q", b.Flag, b.GroupPath)
	}
	if got := order(); got != "bca" {
		t.Errorf("after wake order = %q, want bca", got)
	}

	// Legacy: a session parked in the old global group goes home, still snoozed.
	d := inst("d")
	d.GroupPath = "Work"
	h.instances = append(h.instances, d)
	h.groupTree.AddSession(d)
	d.Snooze(time.Now().Add(time.Hour))
	d.SnoozeFromGroup = "Work"
	h.groupTree.MoveSessionToGroup(d, session.SnoozedGroupPath)
	if !h.migrateLegacySnoozed() {
		t.Fatal("migration should report a change")
	}
	if d.GroupPath != "Work" || d.Flag != session.FlagSnoozed || d.SnoozeFromGroup != "" {
		t.Errorf("migrated: group=%q flag=%v from=%q", d.GroupPath, d.Flag, d.SnoozeFromGroup)
	}
	if _, ok := h.groupTree.Groups[session.SnoozedGroupPath]; ok {
		t.Error("legacy Snoozed group should be gone")
	}
	if h.migrateLegacySnoozed() {
		t.Error("second migration should be a no-op")
	}
}

// The row marker: a clock for today, a calendar beyond, and both two cells
// wide so the separator logic in renderSession keeps titles aligned.
func TestSnoozeIcon(t *testing.T) {
	now := time.Date(2026, 10, 3, 16, 0, 0, 0, time.Local)
	if got := snoozeIcon(now.Add(time.Hour), now); got != "⏰" {
		t.Errorf("today: got %q", got)
	}
	if got := snoozeIcon(now.AddDate(0, 0, 1), now); got != "📅" {
		t.Errorf("tomorrow: got %q", got)
	}
	for _, ic := range []string{"⏰", "📅"} {
		if w := lipgloss.Width(ic); w != 2 {
			t.Errorf("%q width = %d, want 2", ic, w)
		}
	}
}

// The field has focus on open and typing goes straight in; Enter takes a
// typed time that parses, falls back to the first matching suggestion when
// the text is a label fragment, stays open with an error when nothing fits,
// and ↓/↑ walk the suggestions. Esc closes without a message.
func TestSnoozeDialogPicks(t *testing.T) {
	typeKeys := func(d *SnoozeDialog, s string) *SnoozeDialog {
		for _, r := range s {
			d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		return d
	}
	pick := func(t *testing.T, d *SnoozeDialog) (*SnoozeDialog, snoozeSetMsg) {
		t.Helper()
		d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatalf("enter should emit a command (err=%q)", d.err)
		}
		msg, ok := cmd().(snoozeSetMsg)
		if !ok || msg.instanceID != "sid" {
			t.Fatalf("got %#v, want snoozeSetMsg for sid", cmd())
		}
		if d.IsVisible() {
			t.Error("dialog should close after a pick")
		}
		return d, msg
	}

	d := NewSnoozeDialog()
	d.SetSize(100, 40)
	d.Show("sid", "notes")
	if d.cursor != -1 || !d.input.Focused() {
		t.Fatalf("field should have focus on open: cursor=%d focused=%v", d.cursor, d.input.Focused())
	}
	if len(d.suggestions) == 0 {
		t.Fatal("no suggestions built")
	}

	// Typed time wins.
	d = typeKeys(d, "2h")
	if d.input.Value() != "2h" {
		t.Fatalf("typing should land in the field, got %q", d.input.Value())
	}
	d, msg := pick(t, d)
	if dl := msg.until.Sub(time.Now()); dl < 119*time.Minute || dl > 121*time.Minute {
		t.Errorf("2h landed %v out", dl)
	}

	if msg.pick != "typed" {
		t.Errorf("typed pick label = %q, want typed", msg.pick)
	}

	// A label fragment picks the first matching suggestion and reports it.
	d.Show("sid", "notes")
	d = typeKeys(d, "week")
	rows := d.filtered()
	if len(rows) == 0 {
		t.Fatal("expected rows matching 'week' (Later this week / This weekend / In 2 weeks)")
	}
	d, msg = pick(t, d)
	if !msg.until.Equal(rows[0].At) || msg.pick != rows[0].Label {
		t.Errorf("fragment should pick %q (%v), got %q %v", rows[0].Label, rows[0].At, msg.pick, msg.until)
	}

	// Empty field + Enter takes the first suggestion.
	d.Show("sid", "notes")
	first := d.suggestions[0]
	d, msg = pick(t, d)
	if !msg.until.Equal(first.At) {
		t.Errorf("empty enter should pick %q, got %v", first.Label, msg.until)
	}

	// ↓ walks the list; ↑ from the top returns to the field; typing returns too.
	d.Show("sid", "notes")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.cursor != 1 || d.input.Focused() {
		t.Errorf("two downs: cursor=%d focused=%v", d.cursor, d.input.Focused())
	}
	second := d.suggestions[1]
	d, msg = pick(t, d)
	if !msg.until.Equal(second.At) {
		t.Errorf("highlighted row should win, got %v want %v", msg.until, second.At)
	}
	d.Show("sid", "notes")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.cursor != -1 || !d.input.Focused() {
		t.Errorf("up from the top row should refocus the field: cursor=%d", d.cursor)
	}
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	d = typeKeys(d, "7")
	if d.cursor != -1 || d.input.Value() != "7" {
		t.Errorf("typing on a row should return to the field: cursor=%d value=%q", d.cursor, d.input.Value())
	}

	// Nothing fits: stays open with an error.
	d.Show("sid", "notes")
	d = typeKeys(d, "soon")
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !d.IsVisible() || d.err == "" {
		t.Errorf("unreadable entry should stay open with an error: cmd=%v visible=%v err=%q", cmd != nil, d.IsVisible(), d.err)
	}

	d.Show("sid", "notes")
	d, cmd = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || d.IsVisible() {
		t.Error("esc should close silently")
	}
}

// Every snoozed session, from every group, sits in one fold row at the bottom
// of the list, closed by default. Opening it lists them soonest wake first;
// left closes it. The sessions keep their own groups throughout.
func TestSnoozeFold(t *testing.T) {
	a, b, c, d, e := inst("a"), inst("b"), inst("c"), inst("d"), inst("e")
	for i, s := range []*session.Instance{a, b, c} {
		s.GroupPath, s.Order = "Work", i
	}
	d.GroupPath, e.GroupPath, e.Order = "Home", "Home", 1
	h := &Home{instances: []*session.Instance{a, b, c, d, e}}
	h.groupTree = session.NewGroupTree(h.instances)
	h.snoozeSession(a, time.Now().Add(48*time.Hour))
	h.snoozeSession(b, time.Now().Add(2*time.Hour))
	h.snoozeSession(d, time.Now().Add(24*time.Hour))

	rows := func() string {
		out := ""
		for _, it := range h.flatItems {
			switch it.Type {
			case session.ItemTypeGroup:
				out += "G"
			case session.ItemTypeSnoozeFold:
				out += fmt.Sprintf("[%d]", it.SnoozedCount)
			case session.ItemTypeSession:
				out += it.Session.ID
			}
		}
		return out
	}
	// Groups sort by name: Home (e live), Work (c live), then the one fold.
	if got := rows(); got != "GeGc[3]" {
		t.Fatalf("closed: %q, want GeGc[3]", got)
	}
	// A group header counts only the rows the group can show.
	if w, hm := h.groupTree.LiveSessionCountForGroup("Work"), h.groupTree.LiveSessionCountForGroup("Home"); w != 1 || hm != 1 {
		t.Errorf("live counts: Work %d, Home %d, want 1 and 1", w, hm)
	}
	if got := h.groupTree.SessionCountForGroup("Work"); got != 3 {
		t.Errorf("total count for Work: %d, want 3 (the CLI's count is unchanged)", got)
	}
	fold := len(h.flatItems) - 1
	if !h.flatItems[fold-1].IsLastInGroup {
		t.Error("a group's last live session draws as its last line: the fold is no longer in the group")
	}
	if got := h.flatItems[fold]; got.Level != 0 || got.Path != session.SnoozeFoldKey {
		t.Errorf("fold row: level %d path %q, want the list's left edge and the fold key", got.Level, got.Path)
	}
	if !h.flatItems[fold].SnoozeNextWake.Equal(b.SnoozeUntil) {
		t.Error("fold row should carry the soonest wake (b)")
	}

	h.cursor = fold
	h.setSnoozeFold(session.SnoozeFoldKey, true)
	if got := rows(); got != "GeGc[3]bda" {
		t.Fatalf("open: %q, want GeGc[3]bda (soonest wake first, across groups)", got)
	}
	if h.cursor != fold {
		t.Errorf("cursor %d, want the fold row %d", h.cursor, fold)
	}
	for i, want := range []string{"Work", "Home", "Work"} {
		if got := h.flatItems[fold+1+i].Path; got != want {
			t.Errorf("row %d in the fold: path %q, want its own group %q", i, got, want)
		}
	}
	if a.GroupPath != "Work" || d.GroupPath != "Home" {
		t.Error("the bucket is display only: a snoozed session keeps its group")
	}
	if !h.flatItems[len(h.flatItems)-1].IsLastInGroup {
		t.Error("the last row in the fold draws as its last line")
	}

	// A collapsed group does not hide its snoozed sessions from the bucket.
	h.groupTree.CollapseGroup("Work")
	h.rebuildFlatItems()
	if got := rows(); got != "GeG[3]bda" {
		t.Errorf("Work collapsed: %q, want GeG[3]bda", got)
	}
	h.groupTree.ExpandGroup("Work")

	h.setSnoozeFold(session.SnoozeFoldKey, false)
	if got := rows(); got != "GeGc[3]" {
		t.Errorf("closed again: %q", got)
	}

	// Waking a session sends it home and the fold shrinks; the last one out
	// removes the row.
	b.WakeFromSnooze()
	h.rebuildFlatItems()
	if got := rows(); got != "GeGbc[2]" {
		t.Errorf("b woken: %q, want GeGbc[2]", got)
	}
	a.WakeFromSnooze()
	d.WakeFromSnooze()
	h.rebuildFlatItems()
	if got := rows(); got != "GdeGabc" {
		t.Errorf("all woken: %q, want GdeGabc and no fold row", got)
	}
}
