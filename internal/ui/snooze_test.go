package ui

import (
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

// Snoozing moves the session to a Snoozed group pinned at the bottom; waking
// or cancelling returns it to the group it came from and removes the Snoozed
// group once empty. A session moved out of Snoozed by hand stays put.
func TestSnoozeGroupRoundTrip(t *testing.T) {
	a, b := inst("a"), inst("b")
	a.GroupPath, b.GroupPath = "Work", "Work"
	h := &Home{instances: []*session.Instance{a, b}}
	h.groupTree = session.NewGroupTree(h.instances)
	h.groupTree.CreateGroup("Zed") // sorts after "Work" by name; Snoozed must still be last

	h.snoozeSession(a, time.Now().Add(time.Hour))
	if a.GroupPath != session.SnoozedGroupPath || a.SnoozeFromGroup != "Work" {
		t.Fatalf("after snooze: group=%q from=%q", a.GroupPath, a.SnoozeFromGroup)
	}
	last := h.groupTree.GroupList[len(h.groupTree.GroupList)-1]
	if last.Path != session.SnoozedGroupPath {
		t.Errorf("Snoozed group should sort last, got %q", last.Path)
	}

	// Re-snooze keeps the original home group.
	h.snoozeSession(a, time.Now().Add(2*time.Hour))
	if a.SnoozeFromGroup != "Work" {
		t.Errorf("re-snooze lost the home group: %q", a.SnoozeFromGroup)
	}

	// Wake: back to Work, Snoozed group gone.
	a.SnoozeUntil = time.Now().Add(-time.Second)
	woken := h.wakeDueSnoozes(time.Now())
	for _, w := range woken {
		h.returnFromSnooze(w)
	}
	if a.GroupPath != "Work" || a.Flag != session.FlagUnread || a.SnoozeFromGroup != "" {
		t.Errorf("after wake: group=%q flag=%v from=%q", a.GroupPath, a.Flag, a.SnoozeFromGroup)
	}
	if _, ok := h.groupTree.Groups[session.SnoozedGroupPath]; ok {
		t.Error("empty Snoozed group should be removed")
	}

	// Manual move out of Snoozed is respected on wake.
	h.snoozeSession(b, time.Now().Add(time.Hour))
	h.groupTree.MoveSessionToGroup(b, "Zed")
	b.WakeFromSnooze()
	h.returnFromSnooze(b)
	if b.GroupPath != "Zed" {
		t.Errorf("manual move should stick, got %q", b.GroupPath)
	}
	if _, ok := h.groupTree.Groups[session.SnoozedGroupPath]; ok {
		t.Error("Snoozed group left behind after manual move")
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

// Enter on a preset emits the wake time; Enter on an unreadable custom entry
// stays open with an inline error; Esc closes without a message.
func TestSnoozeDialogPicks(t *testing.T) {
	d := NewSnoozeDialog()
	d.SetSize(100, 40)
	d.Show("sid", "notes")

	// Preset row 0 = "1 hour"
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a preset should emit a command")
	}
	msg, ok := cmd().(snoozeSetMsg)
	if !ok || msg.instanceID != "sid" {
		t.Fatalf("got %#v, want snoozeSetMsg for sid", cmd())
	}
	if d := msg.until.Sub(time.Now()); d < 59*time.Minute || d > 61*time.Minute {
		t.Errorf("1 hour preset landed %v out", d)
	}
	if d.IsVisible() {
		t.Error("dialog should close after a pick")
	}

	// Typing on a preset row jumps to the custom row and takes the keys.
	d.Show("sid", "notes")
	for _, r := range "soon" {
		d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if d.cursor != d.customRow() || d.input.Value() != "soon" {
		t.Fatalf("typing should move to custom row: cursor=%d value=%q", d.cursor, d.input.Value())
	}
	d, cmd = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !d.IsVisible() || d.err == "" {
		t.Errorf("bad custom entry should stay open with an error: cmd=%v visible=%v err=%q", cmd != nil, d.IsVisible(), d.err)
	}

	// A readable custom entry works.
	d.input.SetValue("2h")
	d, cmd = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a valid custom entry should emit")
	}
	if m, ok := cmd().(snoozeSetMsg); !ok || m.until.Before(time.Now().Add(119*time.Minute)) {
		t.Errorf("custom 2h: got %#v", cmd())
	}

	d.Show("sid", "notes")
	d, cmd = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || d.IsVisible() {
		t.Error("esc should close silently")
	}
}
