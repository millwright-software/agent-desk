package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/millwright-software/agent-desk/internal/statedb"
)

func newTestInbox(t *testing.T) (*HookInbox, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hooks.db")
	inbox, err := statedb.OpenHookInbox(path)
	if err != nil {
		t.Fatal(err)
	}
	h := &HookInbox{inbox: inbox, statuses: map[string]*HookStatus{}}
	t.Cleanup(h.Stop)
	return h, path
}

func TestHookInbox_RefreshThenGet(t *testing.T) {
	h, path := newTestInbox(t)

	// A handler process writes through its own handle.
	writer, err := statedb.OpenHookInbox(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	at := time.Now().Truncate(time.Second)
	if err := writer.Record(statedb.HookEvent{InstanceID: "inst-1", Status: "waiting", Event: "PermissionRequest", SessionID: "sess", At: at}); err != nil {
		t.Fatal(err)
	}

	if got := h.GetHookStatus("inst-1"); got != nil {
		t.Fatalf("before Refresh the snapshot is empty, got %+v", got)
	}
	h.Refresh()
	got := h.GetHookStatus("inst-1")
	if got == nil || got.Status != "waiting" || got.Event != "PermissionRequest" || got.SessionID != "sess" || !got.UpdatedAt.Equal(at) {
		t.Fatalf("got %+v", got)
	}
	if h.GetHookStatus("nope") != nil {
		t.Error("unknown instance should be nil")
	}
}

// Files from the pre-inbox versions are imported once, newest wins, and
// garbage is skipped.
func TestHookInbox_ImportsLegacyFiles(t *testing.T) {
	h, _ := newTestInbox(t)
	dir := t.TempDir()
	write := func(name string, v any) {
		t.Helper()
		data, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("old-1.json", legacyHookFile{Status: "dead", SessionID: "s", Event: "SessionEnd", Timestamp: 1700000000})
	write("old-2.json", legacyHookFile{Status: "waiting", Event: "Stop", Timestamp: 1700000500})
	_ = os.WriteFile(filepath.Join(dir, "junk.json"), []byte("{not json"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0644)

	// The table already knows a NEWER event for old-1: the file must not win.
	if err := h.inbox.Record(statedb.HookEvent{InstanceID: "old-1", Status: "running", Event: "UserPromptSubmit", At: time.Unix(1700009999, 0)}); err != nil {
		t.Fatal(err)
	}

	h.importLegacyFiles(dir)
	h.Refresh()
	if got := h.GetHookStatus("old-1"); got == nil || got.Event != "UserPromptSubmit" {
		t.Errorf("newer table row should survive the import, got %+v", got)
	}
	if got := h.GetHookStatus("old-2"); got == nil || got.Status != "waiting" || got.UpdatedAt.Unix() != 1700000500 {
		t.Errorf("old-2 should be imported, got %+v", got)
	}
	if h.GetHookStatus("junk") != nil {
		t.Error("unparseable file must not become a row")
	}
	// The files are left alone.
	if _, err := os.Stat(filepath.Join(dir, "old-2.json")); err != nil {
		t.Error("import must not delete legacy files")
	}
}
