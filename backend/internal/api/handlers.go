package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/sessionmap"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
)

// evalSymlinks resolves symlinks, falling back to the original path on error.
func evalSymlinks(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// jsonResponse writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ServiceStatus describes the health of a single dependency.
type ServiceStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
	Instances *int   `json:"instances,omitempty"` // only set for agent
}

// DetailedHealth is the response for GET /api/health/detailed.
type DetailedHealth struct {
	// Overall is "green" (all ok), "yellow" (optional services down), or "red" (agent down).
	Overall  string          `json:"overall"`
	Services []ServiceStatus `json:"services"`
}

func (s *Server) handleHealthDetailed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var services []ServiceStatus

	// 1. Agent (OpenCode) — check binary availability + instance count.
	// With the provider registry, zero running instances is normal (on-demand).
	agentStatus := ServiceStatus{Name: "agent"}
	instanceCount := s.registry.InstanceCount()
	agentStatus.Instances = &instanceCount

	if err := s.registry.Ready(ctx); err != nil {
		agentStatus.Available = false
		agentStatus.Error = err.Error()
	} else {
		agentStatus.Available = true
	}
	services = append(services, agentStatus)

	// 2. TTS — optional (voice feature)
	ttsStatus := ServiceStatus{Name: "tts"}
	if s.ttsProvider == nil {
		ttsStatus.Available = false
		ttsStatus.Error = "not configured"
	} else {
		_, err := s.ttsProvider.ListVoices(ctx)
		if err != nil {
			ttsStatus.Available = false
			ttsStatus.Error = err.Error()
		} else {
			ttsStatus.Available = true
		}
	}
	services = append(services, ttsStatus)

	// 3. STT — optional (voice feature)
	sttStatus := ServiceStatus{Name: "stt"}
	if s.sttProvider == nil {
		sttStatus.Available = false
		sttStatus.Error = "not configured"
	} else {
		sttStatus.Available = s.sttProvider.HealthCheck(ctx)
		if !sttStatus.Available {
			sttStatus.Error = "health check failed"
		}
	}
	services = append(services, sttStatus)

	// Determine overall status
	overall := "green"
	if !ttsStatus.Available || !sttStatus.Available {
		overall = "yellow"
	}
	if !agentStatus.Available {
		overall = "red"
	}

	jsonResponse(w, http.StatusOK, DetailedHealth{
		Overall:  overall,
		Services: services,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	adapter, err := s.anyAdapter()
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "no running agent instances")
		return
	}
	status, err := adapter.GetStatus(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, status)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	adapter, err := s.anyAdapter()
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "no running agent instances")
		return
	}
	agents, err := adapter.ListAgents(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, agents)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	adapter, err := s.anyAdapter()
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "no running agent instances")
		return
	}
	models, err := adapter.ListModels(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, models)
}

// sessionResponse wraps a session with the titleOverride flag.
type sessionResponse struct {
	agent.Session
	TitleOverride bool `json:"titleOverride"`
}

func (s *Server) applyTitleOverride(session *agent.Session) sessionResponse {
	resp := sessionResponse{Session: *session}
	if s.sessionMap != nil {
		if t := s.sessionMap.GetTitle(session.ID); t != "" {
			resp.Title = t
			resp.TitleOverride = true
		}
	}
	return resp
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	worktreePath := r.URL.Query().Get("worktree")
	if worktreePath == "" {
		jsonError(w, http.StatusBadRequest, "worktree query parameter is required")
		return
	}
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		providerName = s.resolveProviderForWorktree(worktreePath)
	}
	adapter, err := s.resolveAdapterForWorktree(r, worktreePath, providerName)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "failed to get agent instance: "+err.Error())
		return
	}
	sessions, err := adapter.ListSessions(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sessions = agent.FilterTopLevel(sessions)

	// Filter sessions to only those mapped to this worktree path.
	// OpenCode may return sessions for the entire project (shared across
	// git worktrees), so we use the session map as the source of truth.
	resolvedWT := evalSymlinks(worktreePath)
	if s.sessionMap != nil {
		filtered := sessions[:0]
		for i := range sessions {
			entry := s.sessionMap.GetEntry(sessions[i].ID)
			if entry.WorktreePath != "" && evalSymlinks(entry.WorktreePath) == resolvedWT {
				filtered = append(filtered, sessions[i])
			}
		}
		sessions = filtered
	}

	result := make([]sessionResponse, len(sessions))
	for i := range sessions {
		result[i] = s.applyTitleOverride(&sessions[i])
	}
	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var opts agent.SessionOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if opts.Mode == "" {
		opts.Mode = agent.ModePlan
	}
	// Default agent based on mode if not explicitly set
	if opts.Agent == "" {
		if opts.Mode == agent.ModePlan {
			opts.Agent = "planitect"
		} else {
			opts.Agent = "build"
		}
	}
	if opts.WorktreePath == "" {
		jsonError(w, http.StatusBadRequest, "worktreePath is required")
		return
	}
	// Resolve provider: explicit > worktree default > global default
	providerName := opts.Provider
	if providerName == "" {
		providerName = s.resolveProviderForWorktree(opts.WorktreePath)
	}
	adapter, err := s.resolveAdapterForWorktree(r, opts.WorktreePath, providerName)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "failed to get agent instance: "+err.Error())
		return
	}
	session, err := adapter.CreateSession(r.Context(), opts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Map session to worktree in the session map for lookup
	if opts.WorktreePath != "" && s.sessionMap != nil {
		var ts int64
		if session.Time != nil && session.Time.Created > 0 {
			ts = session.Time.Created
		}
		if err := s.sessionMap.SetEntry(session.ID, sessionmap.Entry{
			WorktreePath: opts.WorktreePath,
			Provider:     providerName,
			UpdatedAt:    ts,
		}); err != nil {
			jsonError(w, http.StatusInternalServerError, "session created but mapping failed: "+err.Error())
			return
		}
	}

	// Save worktree default provider (last-used pattern)
	if providerName != "" && s.wtDefaults != nil {
		if err := s.wtDefaults.Set(opts.WorktreePath, providerName); err != nil {
			slog.Warn("failed to save worktree default provider", "error", err)
		}
	}

	// Initialize session with scoping prompt (fire-and-forget)
	prompt := agent.ScopePrompt(opts.WorktreePath)
	if err := adapter.InitializeSession(r.Context(), session.ID, prompt); err != nil {
		slog.Warn("session initialization failed", "sessionID", session.ID, "error", err)
	}

	jsonResponse(w, http.StatusCreated, session)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	adapter, err := s.resolveAdapter(r, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}
	if err := adapter.DeleteSession(r.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	adapter, err := s.resolveAdapter(r, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}
	session, err := adapter.ResumeSession(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}
	if !session.IsTopLevel() {
		jsonError(w, http.StatusNotFound, "session not found")
		return
	}

	// Wrap the session with a busy flag and title override.
	resp := s.applyTitleOverride(session)
	type sessionWithStatus struct {
		sessionResponse
		Busy bool `json:"busy"`
	}
	jsonResponse(w, http.StatusOK, sessionWithStatus{
		sessionResponse: resp,
		Busy:            adapter.GetSessionBusy(id),
	})
}

func (s *Server) handleGetSessionMessages(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	adapter, err := s.resolveAdapter(r, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}
	messages, err := adapter.GetMessages(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to get messages: "+err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, messages)
}

func (s *Server) handleSetSessionTitle(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(body.Title) > 80 {
		jsonError(w, http.StatusBadRequest, "title must be 80 characters or less")
		return
	}

	if err := s.sessionMap.SetTitle(id, body.Title); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save title: "+err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":            id,
		"title":         body.Title,
		"titleOverride": body.Title != "",
	})
}

func (s *Server) handleSetSessionMode(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	mode := agent.SessionMode(body.Mode)
	if mode != agent.ModePlan && mode != agent.ModeImplement {
		jsonError(w, http.StatusBadRequest, "invalid mode: must be 'plan' or 'implement'")
		return
	}

	adapter, err := s.resolveAdapter(r, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}
	adapter.SetSessionMode(id, mode)
	jsonResponse(w, http.StatusOK, map[string]string{"mode": string(mode)})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// Limit message body to 1MB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	adapter, err := s.resolveAdapter(r, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}

	events, err := adapter.SendMessage(r.Context(), id, body.Message)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Stream events as Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	for event := range events {
		data, _ := json.Marshal(event)
		w.Write([]byte("data: "))
		w.Write(data)
		w.Write([]byte("\n\n"))
		flusher.Flush()
	}
}

func (s *Server) handleTTSSynthesize(w http.ResponseWriter, r *http.Request) {
	if s.ttsProvider == nil {
		jsonError(w, http.StatusServiceUnavailable, "TTS not configured")
		return
	}

	var req tts.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := s.ttsProvider.Synthesize(r.Context(), req)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Audio.Close()

	w.Header().Set("Content-Type", resp.ContentType)
	io.Copy(w, resp.Audio)
}

func (s *Server) handleTTSVoices(w http.ResponseWriter, r *http.Request) {
	if s.ttsProvider == nil {
		jsonError(w, http.StatusServiceUnavailable, "TTS not configured")
		return
	}

	voices, err := s.ttsProvider.ListVoices(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, voices)
}

func (s *Server) handleAbortSession(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	adapter, err := s.resolveAdapter(r, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}
	if err := adapter.AbortSession(r.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSTTTranscribe(w http.ResponseWriter, r *http.Request) {
	if s.sttProvider == nil {
		jsonError(w, http.StatusServiceUnavailable, "STT not configured")
		return
	}

	contentType := r.Header.Get("Content-Type")
	var audio io.Reader
	var audioContentType string

	// If the client sends multipart/form-data, extract the 'audio' file field.
	// Otherwise, treat the raw body as the audio stream.
	if r.MultipartForm != nil || strings.HasPrefix(contentType, "multipart/") {
		if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB max
			jsonError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
			return
		}
		file, header, err := r.FormFile("audio")
		if err != nil {
			jsonError(w, http.StatusBadRequest, "missing 'audio' field in multipart form: "+err.Error())
			return
		}
		defer file.Close()
		audio = file
		audioContentType = header.Header.Get("Content-Type")
		if audioContentType == "" {
			// Guess from filename
			name := header.Filename
			switch {
			case len(name) > 4 && name[len(name)-4:] == ".wav":
				audioContentType = "audio/wav"
			case len(name) > 5 && name[len(name)-5:] == ".webm":
				audioContentType = "audio/webm"
			case len(name) > 4 && name[len(name)-4:] == ".ogg":
				audioContentType = "audio/ogg"
			case len(name) > 4 && name[len(name)-4:] == ".mp3":
				audioContentType = "audio/mp3"
			default:
				audioContentType = "audio/wav"
			}
		}
	} else {
		audio = r.Body
		audioContentType = contentType
	}

	result, err := s.sttProvider.Transcribe(r.Context(), stt.Request{
		Audio:       audio,
		ContentType: audioContentType,
	})
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, result)
}

// handleListInstances returns all running agent backend instances.
func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	instances := s.registry.ListInstances()
	type instanceInfo struct {
		WorkDir      string `json:"workDir"`
		Provider     string `json:"provider"`
		BaseURL      string `json:"baseURL"`
		PID          int    `json:"pid"`
		LastActivity string `json:"lastActivity"`
		Idle         bool   `json:"idle"`
	}
	out := make([]instanceInfo, len(instances))
	for i, inst := range instances {
		out[i] = instanceInfo{
			WorkDir:      inst.WorkDir,
			Provider:     inst.ProviderName,
			BaseURL:      inst.BaseURL,
			PID:          inst.PID,
			LastActivity: inst.LastActivity.Format(time.RFC3339),
			Idle:         inst.IsIdle(),
		}
	}
	jsonResponse(w, http.StatusOK, out)
}

// handleStopInstance stops a running agent instance by worktree path and provider.
func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkDir  string `json:"workDir"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkDir == "" {
		jsonError(w, http.StatusBadRequest, "workDir is required")
		return
	}
	if req.Provider == "" {
		req.Provider = s.registry.DefaultProviderName()
	}
	if err := s.registry.StopInstance(req.WorkDir, req.Provider); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleListProviders returns all configured provider names and the default.
func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"providers":       s.registry.ProviderNames(),
		"defaultProvider": s.registry.DefaultProviderName(),
	})
}

// handleGetWorktreeDefault returns the default provider for a worktree.
func (s *Server) handleGetWorktreeDefault(w http.ResponseWriter, r *http.Request) {
	worktreePath := r.URL.Query().Get("worktree")
	if worktreePath == "" {
		jsonError(w, http.StatusBadRequest, "worktree query parameter is required")
		return
	}
	provider := s.resolveProviderForWorktree(worktreePath)
	jsonResponse(w, http.StatusOK, map[string]string{"provider": provider})
}

// handleSetWorktreeDefault sets the default provider for a worktree.
func (s *Server) handleSetWorktreeDefault(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorktreePath string `json:"worktreePath"`
		Provider     string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorktreePath == "" {
		jsonError(w, http.StatusBadRequest, "worktreePath is required")
		return
	}
	if s.wtDefaults == nil {
		jsonError(w, http.StatusServiceUnavailable, "worktree defaults not configured")
		return
	}
	if err := s.wtDefaults.Set(req.WorktreePath, req.Provider); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save worktree default: "+err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{
		"worktreePath": req.WorktreePath,
		"provider":     req.Provider,
	})
}
