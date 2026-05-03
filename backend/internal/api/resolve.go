package api

import (
	"fmt"
	"net/http"

	"github.com/jenpet/voilot/internal/agent"
)

// resolveAdapter returns the adapter for the given session ID by looking up
// the session's worktree in the session map and getting the adapter from the registry.
func (s *Server) resolveAdapter(r *http.Request, sessionID string) (agent.Adapter, error) {
	if s.sessionMap == nil {
		return nil, fmt.Errorf("session map not configured")
	}
	wtPath := s.sessionMap.Get(sessionID)
	if wtPath == "" {
		return nil, fmt.Errorf("no worktree mapping for session %s", sessionID)
	}
	return s.registry.GetOrSpawn(r.Context(), wtPath)
}

// resolveAdapterForWorktree returns the adapter for the given worktree path.
func (s *Server) resolveAdapterForWorktree(r *http.Request, worktreePath string) (agent.Adapter, error) {
	return s.registry.GetOrSpawn(r.Context(), worktreePath)
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
