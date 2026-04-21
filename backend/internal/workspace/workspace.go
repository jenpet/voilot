// Package workspace discovers projects and git worktrees from a flat
// workspace directory. It scans one level deep, uses git metadata to
// distinguish main repos from linked worktrees, and groups them into
// a Project → Worktree hierarchy.
package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Project represents a git repository discovered in the workspace.
type Project struct {
	Name         string     `json:"name"`
	Path         string     `json:"path"`                   // absolute path to the main repo
	Worktrees    []Worktree `json:"worktrees"`              // includes main + linked worktrees
	LastActivity int64      `json:"lastActivity,omitempty"` // unix ms of most recent session activity (0 = none)
}

// Worktree represents a single git worktree (main or linked).
type Worktree struct {
	Name   string `json:"name"`   // display name (branch slug or "main")
	Path   string `json:"path"`   // absolute path to this worktree directory
	Branch string `json:"branch"` // current branch name
	IsMain bool   `json:"isMain"` // true for the primary worktree
}

// Scanner discovers projects and worktrees from the workspace directory.
type Scanner struct {
	mu           sync.RWMutex
	workspaceDir string
	projects     []Project
}

// NewScanner creates a scanner for the given workspace directory.
func NewScanner(workspaceDir string) *Scanner {
	return &Scanner{workspaceDir: workspaceDir}
}

// WorkspaceDir returns the configured workspace directory.
func (s *Scanner) WorkspaceDir() string {
	return s.workspaceDir
}

// Scan discovers all projects and worktrees in the workspace directory.
// It updates the internal cache and returns the results.
func (s *Scanner) Scan() ([]Project, error) {
	entries, err := os.ReadDir(s.workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("read workspace dir: %w", err)
	}

	// First pass: classify each directory as main or linked worktree.
	type dirInfo struct {
		path      string
		name      string
		gitCommon string // resolved common git dir
		branch    string
		isMain    bool
	}

	var dirs []dirInfo
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}

		dirPath := filepath.Join(s.workspaceDir, entry.Name())

		// Resolve symlinks
		resolved, err := filepath.EvalSymlinks(dirPath)
		if err != nil {
			continue
		}

		// Check if it's a git repo
		gitDir, err := gitOutput(resolved, "rev-parse", "--git-dir")
		if err != nil {
			continue // not a git repo
		}

		commonDir, err := gitOutput(resolved, "rev-parse", "--git-common-dir")
		if err != nil {
			continue
		}

		// Resolve to absolute paths for reliable comparison
		absGitDir := resolvePath(resolved, gitDir)
		absCommonDir := resolvePath(resolved, commonDir)

		isMain := absGitDir == absCommonDir

		branch, _ := gitOutput(resolved, "rev-parse", "--abbrev-ref", "HEAD")

		dirs = append(dirs, dirInfo{
			path:      dirPath,
			name:      entry.Name(),
			gitCommon: absCommonDir,
			branch:    branch,
			isMain:    isMain,
		})
	}

	// Second pass: group by common git dir to form projects.
	// Main worktrees define projects; linked worktrees attach to them.
	projectMap := make(map[string]*Project) // keyed by gitCommon path
	mainPaths := make(map[string]string)    // gitCommon -> main worktree dir name

	// Register main worktrees first
	for _, d := range dirs {
		if !d.isMain {
			continue
		}
		projectName := d.name
		projectMap[d.gitCommon] = &Project{
			Name: projectName,
			Path: d.path,
			Worktrees: []Worktree{{
				Name:   "main",
				Path:   d.path,
				Branch: d.branch,
				IsMain: true,
			}},
		}
		mainPaths[d.gitCommon] = d.name
	}

	// Attach linked worktrees to their parent projects
	for _, d := range dirs {
		if d.isMain {
			continue
		}
		proj, ok := projectMap[d.gitCommon]
		if !ok {
			// Orphaned worktree — main repo not in workspace. Create a
			// synthetic project entry so it's still visible.
			proj = &Project{
				Name: extractProjectName(d.name),
				Path: d.gitCommon, // best guess
			}
			projectMap[d.gitCommon] = proj
		}

		// Derive display name: strip project prefix (e.g. "voilot.plan-foo" -> "plan-foo")
		displayName := d.name
		if mainName, ok := mainPaths[d.gitCommon]; ok {
			if trimmed := strings.TrimPrefix(d.name, mainName+"."); trimmed != d.name {
				displayName = trimmed
			}
		}

		proj.Worktrees = append(proj.Worktrees, Worktree{
			Name:   displayName,
			Path:   d.path,
			Branch: d.branch,
			IsMain: false,
		})
	}

	// Collect results sorted by name
	var projects []Project
	for _, p := range projectMap {
		projects = append(projects, *p)
	}

	s.mu.Lock()
	s.projects = projects
	s.mu.Unlock()

	return projects, nil
}

// Projects returns the last scan results.
func (s *Scanner) Projects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Project, len(s.projects))
	copy(result, s.projects)
	return result
}

// FindProject returns the project with the given name, or nil.
func (s *Scanner) FindProject(name string) *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.Name == name {
			cp := p
			return &cp
		}
	}
	return nil
}

// gitOutput runs a git command in the given directory and returns trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolvePath resolves a potentially relative path against a base directory.
func resolvePath(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(base, p))
}

// extractProjectName guesses a project name from a worktree directory name
// by taking the part before the first dot.
func extractProjectName(dirName string) string {
	if idx := strings.Index(dirName, "."); idx > 0 {
		return dirName[:idx]
	}
	return dirName
}
