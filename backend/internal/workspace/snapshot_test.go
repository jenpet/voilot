package workspace

import (
	"testing"
)

func TestRenderWelcomeMessage_NilSnapshot(t *testing.T) {
	got := RenderWelcomeMessage(nil, "/tmp/test")
	want := "Working in `/tmp/test`."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderWelcomeMessage_EmptyRepo(t *testing.T) {
	s := &WorktreeSnapshot{Branch: "main", IsEmpty: true}
	got := RenderWelcomeMessage(s, "/tmp/test")
	want := "Fresh repo on `main`, no commits yet."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderWelcomeMessage_CleanOnBranch(t *testing.T) {
	s := &WorktreeSnapshot{
		Branch:            "feat/foo",
		HeadSHA:           "abc1234",
		LastCommitSubject: "Add feature",
	}
	got := RenderWelcomeMessage(s, "/tmp/test")
	want := "You're on `feat/foo`. Working tree is clean. Last commit: *Add feature*."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderWelcomeMessage_DirtyOnBranch(t *testing.T) {
	s := &WorktreeSnapshot{
		Branch:            "main",
		HeadSHA:           "abc1234",
		Modified:          3,
		Untracked:         1,
		LastCommitSubject: "Fix bug",
	}
	got := RenderWelcomeMessage(s, "/tmp/test")
	want := "You're on `main`. 3 modified, 1 untracked files. Last commit: *Fix bug*."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderWelcomeMessage_DetachedHead(t *testing.T) {
	s := &WorktreeSnapshot{
		Branch:            "HEAD",
		HeadSHA:           "a1b2c3d",
		LastCommitSubject: "Initial commit",
	}
	got := RenderWelcomeMessage(s, "/tmp/test")
	want := "Detached at `a1b2c3d`. Working tree is clean. Last commit: *Initial commit*."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderWelcomeMessage_WithUpstream(t *testing.T) {
	s := &WorktreeSnapshot{
		Branch:            "plan/welcome",
		HeadSHA:           "abc1234",
		Upstream:          "origin/plan/welcome",
		CommitsAhead:      2,
		CommitsBehind:     1,
		LastCommitSubject: "Update plan",
	}
	got := RenderWelcomeMessage(s, "/tmp/test")
	want := "You're on `plan/welcome`, 2 ahead / 1 behind `origin/plan/welcome`. Working tree is clean. Last commit: *Update plan*."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderWelcomeMessage_UpstreamInSync(t *testing.T) {
	s := &WorktreeSnapshot{
		Branch:            "main",
		HeadSHA:           "abc1234",
		Upstream:          "origin/main",
		CommitsAhead:      0,
		CommitsBehind:     0,
		LastCommitSubject: "Release",
	}
	got := RenderWelcomeMessage(s, "/tmp/test")
	// No ahead/behind shown when both are 0
	want := "You're on `main`. Working tree is clean. Last commit: *Release*."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderWelcomeMessage_StagedFiles(t *testing.T) {
	s := &WorktreeSnapshot{
		Branch:            "main",
		HeadSHA:           "abc1234",
		Staged:            2,
		LastCommitSubject: "Init",
	}
	got := RenderWelcomeMessage(s, "/tmp/test")
	want := "You're on `main`. 2 staged files. Last commit: *Init*."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
