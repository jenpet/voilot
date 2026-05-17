package workspace

import (
	"strconv"
	"strings"
)

// WorktreeSnapshot holds git state captured at session creation time.
// Stored in the session map and used to render the welcome message.
type WorktreeSnapshot struct {
	Branch            string `json:"branch"`                      // current branch name; "HEAD" if detached
	HeadSHA           string `json:"headSHA,omitempty"`           // short SHA of HEAD
	Staged            int    `json:"staged,omitempty"`            // number of staged files
	Modified          int    `json:"modified,omitempty"`          // number of modified (unstaged) files
	Untracked         int    `json:"untracked,omitempty"`         // number of untracked files
	LastCommitSubject string `json:"lastCommitSubject,omitempty"` // subject line of HEAD commit
	Upstream          string `json:"upstream,omitempty"`          // remote tracking branch (e.g. "origin/main")
	CommitsAhead      int    `json:"commitsAhead,omitempty"`      // commits ahead of upstream
	CommitsBehind     int    `json:"commitsBehind,omitempty"`     // commits behind upstream
	IsEmpty           bool   `json:"isEmpty,omitempty"`           // true if repo has no commits
}

// CollectWorktreeSnapshot runs git commands against the given path and
// returns a snapshot of the worktree state. Individual command failures
// are silently ignored — the snapshot will have zero-values for those fields.
func CollectWorktreeSnapshot(worktreePath string) *WorktreeSnapshot {
	s := &WorktreeSnapshot{}

	// Branch name
	branch, err := gitOutput(worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		// Might be an empty repo — try symbolic-ref
		if ref, refErr := gitOutput(worktreePath, "symbolic-ref", "HEAD"); refErr == nil {
			s.Branch = strings.TrimPrefix(ref, "refs/heads/")
			s.IsEmpty = true
		}
		return s
	}
	s.Branch = branch

	// Short SHA
	if sha, err := gitOutput(worktreePath, "rev-parse", "--short", "HEAD"); err == nil {
		s.HeadSHA = sha
	}

	// Last commit subject
	if subject, err := gitOutput(worktreePath, "log", "-1", "--format=%s"); err == nil {
		s.LastCommitSubject = subject
	} else {
		s.IsEmpty = true
	}

	// Working tree status
	if status, err := gitOutput(worktreePath, "status", "--porcelain"); err == nil && status != "" {
		for _, line := range strings.Split(status, "\n") {
			if len(line) < 2 {
				continue
			}
			x, y := line[0], line[1]
			switch {
			case x == '?' && y == '?':
				s.Untracked++
			case x != ' ' && x != '?':
				s.Staged++
				// A file can be both staged and modified
				if y != ' ' && y != '?' {
					s.Modified++
				}
			case y != ' ' && y != '?':
				s.Modified++
			}
		}
	}

	// Upstream tracking
	if upstream, err := gitOutput(worktreePath, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil && upstream != "" {
		s.Upstream = upstream
		if ahead, err := gitOutput(worktreePath, "rev-list", "--count", upstream+"..HEAD"); err == nil {
			s.CommitsAhead, _ = strconv.Atoi(ahead)
		}
		if behind, err := gitOutput(worktreePath, "rev-list", "--count", "HEAD.."+upstream); err == nil {
			s.CommitsBehind, _ = strconv.Atoi(behind)
		}
	}

	return s
}

// RenderWelcomeMessage produces a short greeting from the snapshot.
// Returns a fallback message if the snapshot is nil.
func RenderWelcomeMessage(snapshot *WorktreeSnapshot, worktreePath string) string {
	if snapshot == nil {
		return "Working in `" + worktreePath + "`."
	}

	if snapshot.IsEmpty {
		if snapshot.Branch != "" {
			return "Fresh repo on `" + snapshot.Branch + "`, no commits yet."
		}
		return "Working in `" + worktreePath + "`."
	}

	var b strings.Builder

	// Branch or detached HEAD
	if snapshot.Branch == "HEAD" {
		b.WriteString("Detached at `" + snapshot.HeadSHA + "`.")
	} else {
		b.WriteString("You're on `" + snapshot.Branch + "`")
		// Upstream info
		if snapshot.Upstream != "" {
			parts := []string{}
			if snapshot.CommitsAhead > 0 {
				parts = append(parts, strconv.Itoa(snapshot.CommitsAhead)+" ahead")
			}
			if snapshot.CommitsBehind > 0 {
				parts = append(parts, strconv.Itoa(snapshot.CommitsBehind)+" behind")
			}
			if len(parts) > 0 {
				b.WriteString(", " + strings.Join(parts, " / ") + " `" + snapshot.Upstream + "`")
			}
		}
		b.WriteString(".")
	}

	// Dirty state
	if snapshot.Staged > 0 || snapshot.Modified > 0 || snapshot.Untracked > 0 {
		parts := []string{}
		if snapshot.Staged > 0 {
			parts = append(parts, strconv.Itoa(snapshot.Staged)+" staged")
		}
		if snapshot.Modified > 0 {
			parts = append(parts, strconv.Itoa(snapshot.Modified)+" modified")
		}
		if snapshot.Untracked > 0 {
			parts = append(parts, strconv.Itoa(snapshot.Untracked)+" untracked")
		}
		b.WriteString(" " + strings.Join(parts, ", ") + " files.")
	} else {
		b.WriteString(" Working tree is clean.")
	}

	// Last commit
	if snapshot.LastCommitSubject != "" {
		b.WriteString(" Last commit: *" + snapshot.LastCommitSubject + "*.")
	}

	return b.String()
}
