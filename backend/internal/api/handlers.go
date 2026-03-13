package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
)

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

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.agentAdapter.GetStatus(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, status)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.agentAdapter.ListSessions(r.Context())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, sessions)
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
	session, err := s.agentAdapter.CreateSession(r.Context(), opts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusCreated, session)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.agentAdapter.DeleteSession(r.Context(), id); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	events, err := s.agentAdapter.SendMessage(r.Context(), id, body.Message)
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
	if err := s.agentAdapter.AbortSession(r.Context(), id); err != nil {
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
	result, err := s.sttProvider.Transcribe(r.Context(), stt.Request{
		Audio:       r.Body,
		ContentType: contentType,
	})
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, result)
}
