package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
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

	// Run wt switch -c <branch> --no-cd --no-hooks from the main repo directory
	cmd := exec.CommandContext(r.Context(), "wt", "switch", "-c", branch, "--no-cd", "--no-hooks")
	cmd.Dir = proj.Path
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

	// Find the worktree to get its branch
	var branch string
	for _, wt := range proj.Worktrees {
		if wt.Name == worktreeName {
			branch = wt.Branch
			break
		}
	}
	if branch == "" {
		jsonError(w, http.StatusNotFound, "worktree not found")
		return
	}

	// Run wt remove from the main repo
	cmd := exec.CommandContext(r.Context(), "wt", "remove", branch)
	cmd.Dir = proj.Path
	if out, err := cmd.CombinedOutput(); err != nil {
		jsonError(w, http.StatusInternalServerError, "wt remove failed: "+string(out))
		return
	}

	// Re-scan
	if _, err := s.scanner.Scan(); err != nil {
		jsonError(w, http.StatusInternalServerError, "re-scan failed: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
	if s.sessionMap == nil {
		jsonError(w, http.StatusServiceUnavailable, "workspace not configured")
		return
	}

	// The path is URL-encoded; gorilla/mux decodes it.
	worktreePath := mux.Vars(r)["path"]
	sessionIDs := s.sessionMap.SessionsForWorktree(worktreePath)
	jsonResponse(w, http.StatusOK, sessionIDs)
}
