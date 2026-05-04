package api

import (
	"fmt"
	"net/http"

	"github.com/jenpet/voilot/internal/agent"
)

// resolveAdapter returns the adapter for the given session ID by looking up
// the session's worktree and provider in the session map.
func (s *Server) resolveAdapter(r *http.Request, sessionID string) (agent.Adapter, error) {
	if s.sessionMap == nil {
		return nil, fmt.Errorf("session map not configured")
	}
	entry := s.sessionMap.GetEntry(sessionID)
	if entry.WorktreePath == "" {
		return nil, fmt.Errorf("no worktree mapping for session %s", sessionID)
	}
	providerName := entry.Provider
	if providerName == "" {
		providerName = s.registry.DefaultProviderName()
	}
	return s.registry.GetOrSpawn(r.Context(), entry.WorktreePath, providerName)
}

// resolveAdapterForWorktree returns the adapter for the given worktree path and provider.
func (s *Server) resolveAdapterForWorktree(r *http.Request, worktreePath, providerName string) (agent.Adapter, error) {
	return s.registry.GetOrSpawn(r.Context(), worktreePath, providerName)
}

// resolveProviderForSession returns the provider name stored for a session,
// falling back to the registry default.
func (s *Server) resolveProviderForSession(sessionID string) string {
	if s.sessionMap != nil {
		if p := s.sessionMap.GetEntry(sessionID).Provider; p != "" {
			return p
		}
	}
	return s.registry.DefaultProviderName()
}

// resolveProviderForWorktree returns the effective provider for a worktree,
// checking worktree defaults then falling back to the registry default.
func (s *Server) resolveProviderForWorktree(worktreePath string) string {
	if s.wtDefaults != nil {
		if p := s.wtDefaults.Get(worktreePath); p != "" {
			return p
		}
	}
	return s.registry.DefaultProviderName()
}

// anyAdapter returns any running adapter, or an error if none are available.
// Used for provider-wide queries like ListAgents or ListModels where the
// answer is the same regardless of which instance we ask.
func (s *Server) anyAdapter() (agent.Adapter, error) {
	instances := s.registry.ListInstances()
	if len(instances) == 0 {
		return nil, fmt.Errorf("no running agent instances")
	}
	return instances[0].Adapter, nil
}
