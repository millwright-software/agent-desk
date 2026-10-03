package statedb

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHookInbox_RecordAllNewestWins(t *testing.T) {
	inbox, err := OpenHookInbox(filepath.Join(t.TempDir(), "hooks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer inbox.Close()

	now := time.Now().Truncate(time.Second)
	must := func(ev HookEvent) {
		t.Helper()
		if err := inbox.Record(ev); err != nil {
			t.Fatal(err)
		}
	}
	must(HookEvent{InstanceID: "a", Status: "running", Event: "UserPromptSubmit", SessionID: "s1", At: now})
	must(HookEvent{InstanceID: "b", Status: "waiting", Event: "Stop", At: now})

	all, err := inbox.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all["a"].Event != "UserPromptSubmit" || all["b"].Status != "waiting" {
		t.Fatalf("unexpected rows: %+v", all)
	}
	if !all["a"].At.Equal(now) {
		t.Errorf("timestamp round trip: got %v want %v", all["a"].At, now)
	}

	// A newer event replaces.
	must(HookEvent{InstanceID: "a", Status: "waiting", Event: "PermissionRequest", SessionID: "s1", At: now.Add(time.Second)})
	// ⚠️ An OLDER event must not move status backwards (legacy import, racing handlers).
	must(HookEvent{InstanceID: "a", Status: "running", Event: "UserPromptSubmit", SessionID: "s1", At: now.Add(-time.Hour)})
	all, _ = inbox.All()
	if all["a"].Event != "PermissionRequest" {
		t.Errorf("stale event overwrote a newer one: %+v", all["a"])
	}
	// Same-second events: the later write wins (>=), as the handler is the
	// only writer and events within one second arrive in order.
	must(HookEvent{InstanceID: "a", Status: "running", Event: "PostToolUse", At: now.Add(time.Second)})
	all, _ = inbox.All()
	if all["a"].Event != "PostToolUse" {
		t.Errorf("same-second later event should win: %+v", all["a"])
	}

	n, _ := inbox.Count()
	if n != 2 {
		t.Errorf("count = %d", n)
	}

	// Prune: keep "a"; "b" is old and unreferenced.
	pruned, err := inbox.Prune(map[string]bool{"a": true}, now.Add(time.Minute))
	if err != nil || pruned != 1 {
		t.Fatalf("prune = %d, %v", pruned, err)
	}
	all, _ = inbox.All()
	if _, ok := all["b"]; ok {
		t.Error("b should be pruned")
	}
}

// Two handles on the same file, as the handler and the TUI are: a write from
// one is visible to the other without reopening.
func TestHookInbox_CrossHandleVisibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.db")
	writer, err := OpenHookInbox(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	reader, err := OpenHookInbox(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := writer.Record(HookEvent{InstanceID: "x", Status: "dead", Event: "SessionEnd", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	all, err := reader.All()
	if err != nil || all["x"].Status != "dead" {
		t.Fatalf("reader did not see the write: %+v %v", all, err)
	}
}
