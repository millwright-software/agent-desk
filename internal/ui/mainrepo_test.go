package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ⚠️ A NEW SESSION MUST NOT DEFAULT INTO A WORKTREE. The group's remembered path
// is the most recent session's, which is frequently a worktree — and a worktree
// is transient: it gets landed and removed when that session ends. Offering it
// as the starting point for the next session points at a directory that often
// no longer exists, or drops the new session inside somebody else's workspace.
func TestNewSessionDefaultResolvesWorktreeToMainRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	must := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "init", "-q", repo).Run()
	must("git", "config", "user.email", "t@t")
	must("git", "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	must("git", "add", "-A")
	must("git", "commit", "-qm", "init")

	wt := filepath.Join(root, "wt")
	must("git", "worktree", "add", "-q", wt, "-b", "feature")

	got := mainRepoForNewSession(wt)
	wantReal, _ := filepath.EvalSymlinks(repo)
	gotReal, _ := filepath.EvalSymlinks(got)
	if gotReal != wantReal {
		t.Errorf("worktree %s resolved to %s, want the main repo %s", wt, got, repo)
	}

	// The main repo resolves to itself — not to something else.
	if got := mainRepoForNewSession(repo); got == "" {
		t.Error("main repo resolved to empty")
	}
}

// ⚠️ A PATH THAT IS NOT IN A REPO MUST COME BACK UNCHANGED. A default that is
// merely unhelpful beats an empty one.
func TestNewSessionDefaultLeavesNonRepoPathsAlone(t *testing.T) {
	plain := t.TempDir()
	if got := mainRepoForNewSession(plain); got != plain {
		t.Errorf("non-repo path %s became %s", plain, got)
	}
	if got := mainRepoForNewSession(""); got != "" {
		t.Errorf("empty path became %q", got)
	}
	if got := mainRepoForNewSession("/no/such/dir/anywhere"); got != "/no/such/dir/anywhere" {
		t.Errorf("missing path became %q", got)
	}
}
