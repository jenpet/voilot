package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
)

// Server is the main HTTP/WebSocket server for voilot.
type Server struct {
	router       *mux.Router
	agentAdapter agent.Adapter
	ttsProvider  tts.Provider
	sttProvider  stt.Provider
}

// NewServer creates a new API server with the given dependencies.
func NewServer(agentAdapter agent.Adapter, ttsProvider tts.Provider, sttProvider stt.Provider) *Server {
	s := &Server{
		router:       mux.NewRouter(),
		agentAdapter: agentAdapter,
		ttsProvider:  ttsProvider,
		sttProvider:  sttProvider,
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
	api.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Agent status
	api.HandleFunc("/status", s.handleStatus).Methods("GET")

	// Sessions
	api.HandleFunc("/sessions", s.handleListSessions).Methods("GET")
	api.HandleFunc("/sessions", s.handleCreateSession).Methods("POST")
	api.HandleFunc("/sessions/{id}", s.handleGetSession).Methods("GET")
	api.HandleFunc("/sessions/{id}", s.handleDeleteSession).Methods("DELETE")
	api.HandleFunc("/sessions/{id}/mode", s.handleSetSessionMode).Methods("PATCH")

	// Messages (REST fallback for non-WebSocket clients)
	api.HandleFunc("/sessions/{id}/message", s.handleSendMessage).Methods("POST")

	// Session control
	api.HandleFunc("/sessions/{id}/abort", s.handleAbortSession).Methods("POST")

	// TTS
	api.HandleFunc("/tts/synthesize", s.handleTTSSynthesize).Methods("POST")
	api.HandleFunc("/tts/voices", s.handleTTSVoices).Methods("GET")

	// STT
	api.HandleFunc("/stt/transcribe", s.handleSTTTranscribe).Methods("POST")

	// WebSocket endpoints
	ws := s.router.PathPrefix("/ws").Subrouter()
	ws.HandleFunc("/chat", s.handleWSChat)
	ws.HandleFunc("/voice", s.handleWSVoice)
}
