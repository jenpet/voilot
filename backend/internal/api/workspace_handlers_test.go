package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanWtMessage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "uncommitted changes with hint",
			in:   "✗ Cannot remove worktree: foo has uncommitted changes\n↳ Commit or stash changes first, or to lose uncommitted changes, run wt remove --force foo\n",
			want: "Cannot remove worktree: foo has uncommitted changes",
		},
		{
			name: "main worktree error",
			in:   "✗ The main worktree cannot be removed\n",
			want: "The main worktree cannot be removed",
		},
		{
			name: "no decorations",
			in:   "plain error no decorations",
			want: "plain error no decorations",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "multiple error lines",
			in:   "✗ First error\n✗ Second error\n↳ hint line\n",
			want: "First error; Second error",
		},
		{
			name: "locked worktree",
			in:   "✗ Cannot remove feature, worktree is locked (editing)\n↳ To unlock, run git worktree unlock /path\n",
			want: "Cannot remove feature, worktree is locked (editing)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanWtMessage(tt.in)
			if got != tt.want {
				t.Errorf("cleanWtMessage(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDirtyFiles(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	// Create a temp git repo.
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Init repo with an initial commit.
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("hello"), 0o644)
	run("add", "existing.txt")
	run("commit", "-m", "initial")

	// Create dirty state:
	// - modified file (unstaged)
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("modified"), 0o644)
	// - untracked file
	os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new"), 0o644)
	// - deleted file (commit it first, then remove)
	os.WriteFile(filepath.Join(dir, "to_delete.txt"), []byte("bye"), 0o644)
	run("add", "to_delete.txt")
	run("commit", "-m", "add to_delete")
	os.Remove(filepath.Join(dir, "to_delete.txt"))
	// - staged new file (add after the commit so it stays staged)
	os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged"), 0o644)
	run("add", "staged.txt")

	files := dirtyFiles(context.Background(), dir)

	// Build a lookup map.
	got := make(map[string]string)
	for _, f := range files {
		got[f["path"]] = f["status"]
	}

	expect := map[string]string{
		"existing.txt":  "modified",
		"untracked.txt": "untracked",
		"staged.txt":    "added",
		"to_delete.txt": "deleted",
	}

	for path, wantStatus := range expect {
		if gotStatus, ok := got[path]; !ok {
			t.Errorf("missing file %q in dirtyFiles output", path)
		} else if gotStatus != wantStatus {
			t.Errorf("file %q: got status %q, want %q", path, gotStatus, wantStatus)
		}
	}
}

func TestDirtyFiles_CleanRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("clean"), 0o644)
	run("add", "file.txt")
	run("commit", "-m", "init")

	files := dirtyFiles(context.Background(), dir)
	if len(files) != 0 {
		t.Errorf("expected no dirty files, got %d: %v", len(files), files)
	}
}
