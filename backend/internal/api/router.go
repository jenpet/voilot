package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/sessionmap"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
	"github.com/jenpet/voilot/internal/workspace"
)

// Server is the main HTTP/WebSocket server for voilot.
type Server struct {
	router      *mux.Router
	registry    *agent.ProviderRegistry
	ttsProvider tts.Provider
	sttProvider stt.Provider
	scanner     *workspace.Scanner
	sessionMap  *sessionmap.Map
	wtDefaults  *agent.WorktreeDefaults
}

// NewServer creates a new API server with the given dependencies.
// scanner, sessionMap, and wtDefaults may be nil when workspace mode is disabled.
func NewServer(registry *agent.ProviderRegistry, ttsProvider tts.Provider, sttProvider stt.Provider, scanner *workspace.Scanner, sessionMap *sessionmap.Map, wtDefaults *agent.WorktreeDefaults) *Server {
	s := &Server{
		router:      mux.NewRouter(),
		registry:    registry,
		ttsProvider: ttsProvider,
		sttProvider: sttProvider,
		scanner:     scanner,
		sessionMap:  sessionMap,
		wtDefaults:  wtDefaults,
	}
	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	api := s.router.PathPrefix("/api").Subrouter()

	// Health check
	api.HandleFunc("/health/detailed", s.handleHealthDetailed).Methods("GET")
	api.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Agent status
	api.HandleFunc("/status", s.handleStatus).Methods("GET")

	// Agents
	api.HandleFunc("/agents", s.handleListAgents).Methods("GET")
	api.HandleFunc("/models", s.handleListModels).Methods("GET")

	// Sessions
	api.HandleFunc("/sessions", s.handleListSessions).Methods("GET")
	api.HandleFunc("/sessions", s.handleCreateSession).Methods("POST")
	api.HandleFunc("/sessions/{id}", s.handleGetSession).Methods("GET")
	api.HandleFunc("/sessions/{id}", s.handleDeleteSession).Methods("DELETE")
	api.HandleFunc("/sessions/{id}/messages", s.handleGetSessionMessages).Methods("GET")
	api.HandleFunc("/sessions/{id}/mode", s.handleSetSessionMode).Methods("PATCH")
	api.HandleFunc("/sessions/{id}/title", s.handleSetSessionTitle).Methods("PATCH")

	// Messages (REST fallback for non-WebSocket clients)
	api.HandleFunc("/sessions/{id}/message", s.handleSendMessage).Methods("POST")

	// Session control
	api.HandleFunc("/sessions/{id}/abort", s.handleAbortSession).Methods("POST")

	// TTS
	api.HandleFunc("/tts/synthesize", s.handleTTSSynthesize).Methods("POST")
	api.HandleFunc("/tts/voices", s.handleTTSVoices).Methods("GET")

	// STT
	api.HandleFunc("/stt/transcribe", s.handleSTTTranscribe).Methods("POST")

	// Workspace (projects & worktrees)
	api.HandleFunc("/projects", s.handleListProjects).Methods("GET")
	api.HandleFunc("/projects", s.handleAddProject).Methods("POST")
	api.HandleFunc("/projects/clone", s.handleCloneProject).Methods("POST")
	api.HandleFunc("/projects/init", s.handleInitProject).Methods("POST")
	api.HandleFunc("/projects/{name}/branches", s.handleListBranches).Methods("GET")
	api.HandleFunc("/projects/{name}/worktrees", s.handleListWorktrees).Methods("GET")
	api.HandleFunc("/projects/{name}/worktrees", s.handleCreateWorktree).Methods("POST")
	api.HandleFunc("/projects/{name}/worktrees/{worktree}", s.handleRemoveWorktree).Methods("DELETE")
	api.HandleFunc("/worktree-sessions", s.handleWorktreeSessions).Methods("GET")

	// Instance management
	api.HandleFunc("/instances", s.handleListInstances).Methods("GET")
	api.HandleFunc("/instances/stop", s.handleStopInstance).Methods("POST")

	// Providers
	api.HandleFunc("/providers", s.handleListProviders).Methods("GET")

	// Worktree default provider
	api.HandleFunc("/worktree-defaults", s.handleGetWorktreeDefault).Methods("GET")
	api.HandleFunc("/worktree-defaults", s.handleSetWorktreeDefault).Methods("PUT")

	// WebSocket endpoints
	ws := s.router.PathPrefix("/ws").Subrouter()
	ws.HandleFunc("/chat", s.handleWSChat)
	ws.HandleFunc("/voice", s.handleWSVoice)
}
