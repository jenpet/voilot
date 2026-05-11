package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/workspace"
)

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	projects, err := s.scanner.Scan()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "scan failed: "+err.Error())
		return
	}

	// Compute last activity per project by joining session map + session timestamps.
	if s.sessionMap != nil {
		// Build sessionID → updatedAt from running instances (overrides stored timestamps).
		liveUpdated := make(map[string]int64)
		for _, inst := range s.registry.ListInstances() {
			sessions, err := inst.Adapter.ListSessions(r.Context())
			if err != nil {
				slog.Warn("could not fetch sessions from instance", "workdir", inst.WorkDir, "error", err)
				continue
			}
			for _, sess := range agent.FilterTopLevel(sessions) {
				if sess.Time != nil && sess.Time.Updated > 0 {
					liveUpdated[sess.ID] = sess.Time.Updated
				} else if sess.Time != nil {
					liveUpdated[sess.ID] = sess.Time.Created
				}
			}
		}

		// For each project, find the max session timestamp across all worktrees.
		// Use session map stored timestamps, overridden by live data when available.
		for i := range projects {
			var maxTs int64
			for j := range projects[i].Worktrees {
				wtPath := projects[i].Worktrees[j].Path

				// Start with the session map's cached last activity.
				wtMax := s.sessionMap.LastActivityForWorktree(wtPath)

				// Override with live data from running instances.
				for _, sid := range s.sessionMap.SessionsForWorktree(wtPath) {
					if ts, ok := liveUpdated[sid]; ok && ts > wtMax {
						wtMax = ts
					}
				}

				projects[i].Worktrees[j].LastActivity = wtMax
				if wtMax > maxTs {
					maxTs = wtMax
				}
			}
			projects[i].LastActivity = maxTs

			// Sort worktrees: root pinned first, then by last activity descending.
			sort.SliceStable(projects[i].Worktrees, func(a, b int) bool {
				wa, wb := projects[i].Worktrees[a], projects[i].Worktrees[b]
				if wa.IsRoot != wb.IsRoot {
					return wa.IsRoot
				}
				return wa.LastActivity > wb.LastActivity
			})
		}
	}

	// Sort: active projects first (most recent first), then inactive alphabetically.
	sort.Slice(projects, func(i, j int) bool {
		a, b := projects[i].LastActivity, projects[j].LastActivity
		if (a > 0) != (b > 0) {
			return a > 0
		}
		if a > 0 {
			return a > b
		}
		return projects[i].Name < projects[j].Name
	})

	jsonResponse(w, http.StatusOK, projects)
}

func (s *Server) handleListWorktrees(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	name := mux.Vars(r)["name"]
	proj := s.scanner.FindProject(name)
	if proj == nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}
	jsonResponse(w, http.StatusOK, proj.Worktrees)
}

// slugify converts a description like "PWA offline support" into "plan/pwa-offline-support".
var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(description string) string {
	s := strings.ToLower(strings.TrimSpace(description))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// BranchInfo describes a git branch with its divergence from the remote tracking branch.
type BranchInfo struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
	Ahead     int    `json:"ahead"`
	Behind    int    `json:"behind"`
	HasRemote bool   `json:"hasRemote"`
}

func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	name := mux.Vars(r)["name"]
	proj := s.scanner.FindProject(name)
	if proj == nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	// Fetch from remote first for accurate divergence info.
	fetchCmd := exec.CommandContext(r.Context(), "git", "fetch", "--quiet")
	fetchCmd.Dir = proj.Path
	_ = fetchCmd.Run() // best-effort; ignore errors (e.g. no remote)

	defaultBranch := workspace.ResolveDefaultBranch(proj.Path)

	// List local branches with upstream tracking info.
	// Format: refname:short | upstream:short | upstream:track
	listCmd := exec.CommandContext(r.Context(), "git", "for-each-ref",
		"--sort=-committerdate",
		"--format=%(refname:short)|%(upstream:short)|%(upstream:track)",
		"refs/heads/",
	)
	listCmd.Dir = proj.Path
	out, err := listCmd.Output()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "git for-each-ref failed")
		return
	}

	var branches []BranchInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		name := parts[0]
		upstream := ""
		track := ""
		if len(parts) > 1 {
			upstream = parts[1]
		}
		if len(parts) > 2 {
			track = parts[2]
		}

		bi := BranchInfo{
			Name:      name,
			IsDefault: name == defaultBranch,
			HasRemote: upstream != "",
		}

		// Parse track info like "[ahead 3, behind 2]" or "[ahead 1]" or "[behind 5]"
		if track != "" {
			track = strings.Trim(track, "[]")
			for _, part := range strings.Split(track, ",") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "ahead ") {
					fmt.Sscanf(part, "ahead %d", &bi.Ahead)
				} else if strings.HasPrefix(part, "behind ") {
					fmt.Sscanf(part, "behind %d", &bi.Behind)
				}
			}
		}

		branches = append(branches, bi)
	}

	// Stable sort: pin default branch first, preserve git's recency order for the rest.
	sort.SliceStable(branches, func(i, j int) bool {
		if branches[i].IsDefault != branches[j].IsDefault {
			return branches[i].IsDefault
		}
		return false
	})

	jsonResponse(w, http.StatusOK, branches)
}

func (s *Server) handleCreateWorktree(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	name := mux.Vars(r)["name"]
	proj := s.scanner.FindProject(name)
	if proj == nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	var body struct {
		Description string `json:"description"` // e.g. "PWA offline support"
		Branch      string `json:"branch"`      // optional override; otherwise slugified from description
		Base        string `json:"base"`        // optional base branch to fork from; defaults to repo default
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	branch := body.Branch
	if branch == "" {
		if body.Description == "" {
			jsonError(w, http.StatusBadRequest, "description or branch required")
			return
		}
		branch = "plan/" + slugify(body.Description)
	}

	// Run wt switch -c <branch> [--base <base>] --no-cd --no-hooks from the main repo directory.
	// Override WORKTRUNK_WORKTREE_PATH so the worktree is created directly in
	// the workspace directory rather than as a sibling of the (possibly
	// symlinked) repo's real path.
	args := []string{"switch", "-c", branch}
	if body.Base != "" {
		args = append(args, "--base", body.Base)
	}
	args = append(args, "--no-cd", "--no-hooks")
	cmd := exec.CommandContext(r.Context(), "wt", args...)
	cmd.Dir = proj.Path
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("WORKTRUNK_WORKTREE_PATH=%s/{{ repo }}.{{ branch | sanitize }}", s.scanner.WorkspaceDir()),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		jsonError(w, http.StatusInternalServerError, "wt switch failed: "+string(out))
		return
	}

	// Re-scan to pick up the new worktree
	projects, err := s.scanner.Scan()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "re-scan failed: "+err.Error())
		return
	}

	// Find the updated project and return it
	for _, p := range projects {
		if p.Name == name {
			jsonResponse(w, http.StatusCreated, p)
			return
		}
	}
	jsonResponse(w, http.StatusCreated, map[string]string{"branch": branch})
}

func (s *Server) handleRemoveWorktree(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	name := mux.Vars(r)["name"]
	worktreeName := mux.Vars(r)["worktree"]

	proj := s.scanner.FindProject(name)
	if proj == nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return
	}

	// Find the worktree to get its branch and path.
	var branch, wtPath string
	for _, wt := range proj.Worktrees {
		if wt.Name == worktreeName {
			branch = wt.Branch
			wtPath = wt.Path
			break
		}
	}
	if branch == "" {
		jsonError(w, http.StatusNotFound, "worktree not found")
		return
	}

	// Build wt remove command; honour ?force=true query param.
	args := []string{"remove", branch}
	if r.URL.Query().Get("force") == "true" {
		args = []string{"remove", "--force", branch}
	}
	cmd := exec.CommandContext(r.Context(), "wt", args...)
	cmd.Dir = proj.Path
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := cleanWtMessage(string(out))
		forceable := strings.Contains(msg, "uncommitted changes")

		resp := map[string]interface{}{
			"error":     msg,
			"forceable": forceable,
		}

		// For forceable errors, include the list of dirty files.
		if forceable && wtPath != "" {
			if files := dirtyFiles(r.Context(), wtPath); len(files) > 0 {
				resp["files"] = files
			}
		}

		jsonResponse(w, http.StatusConflict, resp)
		return
	}

	// Clean up session-map entries pointing to the removed worktree.
	if s.sessionMap != nil && wtPath != "" {
		for _, sid := range s.sessionMap.SessionsForWorktree(wtPath) {
			if err := s.sessionMap.Delete(sid); err != nil {
				slog.Warn("failed to delete session-map entry", "session", sid, "worktree", wtPath, "error", err)
			}
		}
	}

	// Re-scan
	if _, err := s.scanner.Scan(); err != nil {
		jsonError(w, http.StatusInternalServerError, "re-scan failed: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// cleanWtMessage strips the wt CLI decorations (✗ prefix, ↳ hint lines) from
// the output, returning only the primary error message.
func cleanWtMessage(raw string) string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "↳") {
			continue
		}
		line = strings.TrimPrefix(line, "✗ ")
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return strings.TrimSpace(raw)
	}
	return strings.Join(lines, "; ")
}

// dirtyFiles runs git status --porcelain in the given directory and returns
// a list of {path, status} maps for each dirty file.
func dirtyFiles(ctx context.Context, dir string) []map[string]string {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	porcelainMap := map[string]string{
		"M":  "modified",
		"A":  "added",
		"D":  "deleted",
		"R":  "renamed",
		"C":  "copied",
		"??": "untracked",
		"MM": "modified",
		"AM": "modified",
		"UU": "conflict",
	}
	var files []map[string]string
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		status := porcelainMap[code]
		if status == "" {
			status = "modified"
		}
		files = append(files, map[string]string{
			"path":   path,
			"status": status,
		})
	}
	return files
}

func (s *Server) handleAddProject(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	var body struct {
		Path string `json:"path"` // absolute path to an existing git repo
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Path == "" {
		jsonError(w, http.StatusBadRequest, "path is required")
		return
	}

	// Expand ~ to home directory
	p := body.Path
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "cannot resolve home directory")
			return
		}
		p = filepath.Join(home, p[2:])
	}

	// Resolve and validate the path
	absPath, err := filepath.Abs(p)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid path: "+err.Error())
		return
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		jsonError(w, http.StatusBadRequest, "path does not exist or is not a directory")
		return
	}

	// Verify it's a git repository
	cmd := exec.CommandContext(r.Context(), "git", "rev-parse", "--git-dir")
	cmd.Dir = absPath
	if out, err := cmd.CombinedOutput(); err != nil {
		jsonError(w, http.StatusBadRequest, "not a git repository: "+strings.TrimSpace(string(out)))
		return
	}

	// Create symlink: workspace-dir/<basename> -> absPath
	linkName := filepath.Base(absPath)
	linkPath := filepath.Join(s.scanner.WorkspaceDir(), linkName)

	if _, err := os.Lstat(linkPath); err == nil {
		jsonError(w, http.StatusConflict, "project already exists in workspace: "+linkName)
		return
	}

	if err := os.Symlink(absPath, linkPath); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create symlink: "+err.Error())
		return
	}

	// Re-scan to pick up the new project
	projects, err := s.scanner.Scan()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "re-scan failed: "+err.Error())
		return
	}

	// Return the newly added project
	for _, p := range projects {
		if p.Name == linkName {
			jsonResponse(w, http.StatusCreated, p)
			return
		}
	}
	jsonResponse(w, http.StatusCreated, map[string]string{"name": linkName})
}

func (s *Server) handleCloneProject(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	var body struct {
		URL  string `json:"url"`  // git clone URL
		Name string `json:"name"` // optional: override directory name
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.URL == "" {
		jsonError(w, http.StatusBadRequest, "url is required")
		return
	}

	// Derive directory name from URL if not provided
	dirName := body.Name
	if dirName == "" {
		dirName = repoNameFromURL(body.URL)
	}
	if dirName == "" {
		jsonError(w, http.StatusBadRequest, "cannot derive project name from URL; provide 'name' explicitly")
		return
	}

	targetPath := filepath.Join(s.scanner.WorkspaceDir(), dirName)
	if _, err := os.Stat(targetPath); err == nil {
		jsonError(w, http.StatusConflict, "directory already exists: "+dirName)
		return
	}

	cmd := exec.CommandContext(r.Context(), "git", "clone", body.URL, targetPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		jsonError(w, http.StatusInternalServerError, "git clone failed: "+strings.TrimSpace(string(out)))
		return
	}

	projects, err := s.scanner.Scan()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "re-scan failed: "+err.Error())
		return
	}

	for _, p := range projects {
		if p.Name == dirName {
			jsonResponse(w, http.StatusCreated, p)
			return
		}
	}
	jsonResponse(w, http.StatusCreated, map[string]string{"name": dirName})
}

// repoNameFromURL extracts a project name from a git URL.
// e.g. "https://github.com/user/repo.git" -> "repo"
// e.g. "git@github.com:user/repo.git" -> "repo"
func repoNameFromURL(url string) string {
	// Remove trailing .git
	u := strings.TrimSuffix(url, ".git")
	// Remove trailing slash
	u = strings.TrimRight(u, "/")
	// Take the last path segment
	if idx := strings.LastIndexAny(u, "/:"); idx >= 0 {
		return u[idx+1:]
	}
	return u
}

func (s *Server) handleInitProject(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	var body struct {
		Name string `json:"name"` // directory and project name
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	body.Name = slugify(body.Name)
	if body.Name == "" {
		jsonError(w, http.StatusBadRequest, "name results in an empty slug")
		return
	}

	targetPath := filepath.Join(s.scanner.WorkspaceDir(), body.Name)
	if _, err := os.Stat(targetPath); err == nil {
		jsonError(w, http.StatusConflict, "directory already exists: "+body.Name)
		return
	}

	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create directory: "+err.Error())
		return
	}

	cmd := exec.CommandContext(r.Context(), "git", "init")
	cmd.Dir = targetPath
	if out, err := cmd.CombinedOutput(); err != nil {
		// Clean up on failure
		os.RemoveAll(targetPath)
		jsonError(w, http.StatusInternalServerError, "git init failed: "+strings.TrimSpace(string(out)))
		return
	}

	projects, err := s.scanner.Scan()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "re-scan failed: "+err.Error())
		return
	}

	for _, p := range projects {
		if p.Name == body.Name {
			jsonResponse(w, http.StatusCreated, p)
			return
		}
	}
	jsonResponse(w, http.StatusCreated, map[string]string{"name": body.Name})
}

func (s *Server) handleWorktreeSessions(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	worktreePath := r.URL.Query().Get("path")
	if worktreePath == "" {
		jsonError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}
	sessionIDs := s.sessionMap.SessionsForWorktree(worktreePath)
	jsonResponse(w, http.StatusOK, sessionIDs)
}
