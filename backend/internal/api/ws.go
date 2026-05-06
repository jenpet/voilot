package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/voice"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins in development. In production, nginx handles CORS.
		return true
	},
}

// chatInbound is the WebSocket message format from the client.
type chatInbound struct {
	Type         string     `json:"type"`                   // "message", "abort", "set_mode", "set_agent", "set_model", "permission_response", "question_response", "question_reject"
	SessionID    string     `json:"sessionId"`              // target session
	Content      string     `json:"content"`                // message text, mode/agent/model value
	PermissionID string     `json:"permissionId,omitempty"` // for permission_response
	Response     string     `json:"response,omitempty"`     // "once", "always", "reject"
	Remember     bool       `json:"remember,omitempty"`     // persist the permission rule
	QuestionID   string     `json:"questionId,omitempty"`   // for question_response / question_reject
	Answers      [][]string `json:"answers,omitempty"`      // assembled answers array for question_response
}

// chatOutbound is the WebSocket message format to the client.
type chatOutbound struct {
	Type      string                 `json:"type"`                // "event", "command", "error"
	SessionID string                 `json:"sessionId,omitempty"` // source session
	Event     *agent.Event           `json:"event,omitempty"`     // agent event payload
	Content   string                 `json:"content,omitempty"`   // for command/error
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// handleWSChat handles WebSocket connections for real-time text chat.
// The client sends messages; the server subscribes to SSE events and forwards them.
func (s *Server) handleWSChat(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	ctx := r.Context()

	// Subscribe to aggregated SSE events from all agent instances
	eventCh := s.registry.SubscribeEvents(ctx)

	// Write mutex for WebSocket (gorilla websocket is not safe for concurrent writes)
	var writeMu sync.Mutex
	writeJSON := func(msg chatOutbound) {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.WriteJSON(msg); err != nil {
			slog.Error("WebSocket write error", "error", err)
		}
	}

	// Forward SSE events to WebSocket in a goroutine
	go func() {
		for evt := range eventCh {
			writeJSON(chatOutbound{
				Type:      "event",
				SessionID: evt.SessionID,
				Event:     &evt,
			})
		}
	}()

	// Read messages from client
	for {
		var msg chatInbound
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Error("WebSocket read error", "error", err)
			}
			return
		}

		switch msg.Type {
		case "message":
			if msg.SessionID == "" {
				writeJSON(chatOutbound{
					Type:    "error",
					Content: "sessionId is required",
				})
				continue
			}

			// Route through voice command router first
			result := voice.Route(msg.Content)
			if result.Type == voice.CommandApp {
				writeJSON(chatOutbound{
					Type:      "command",
					SessionID: msg.SessionID,
					Content:   string(result.AppCommand),
				})
				continue
			}

			// Send message to the correct adapter instance
			msgText := result.Text
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "No agent instance for session: " + err.Error(),
				})
				continue
			}

			// Send message asynchronously — response events come via SSE
			// Use the session's current agent and model override for routing
			agentName := adapter.GetSessionAgent(msg.SessionID)
			modelID := adapter.GetSessionModel(msg.SessionID)
			if err := adapter.SendMessageAsync(ctx, msg.SessionID, msgText, agentName, modelID); err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "Failed to send message: " + err.Error(),
				})
			} else if s.sessionMap != nil {
				// Update last activity timestamp for sorting.
				s.sessionMap.UpdateTimestamp(msg.SessionID, time.Now().UnixMilli())
			}

		case "abort":
			if msg.SessionID == "" {
				continue
			}
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				continue
			}
			if err := adapter.AbortSession(ctx, msg.SessionID); err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "Failed to abort: " + err.Error(),
				})
			}

		case "set_mode":
			if msg.SessionID == "" {
				writeJSON(chatOutbound{
					Type:    "error",
					Content: "sessionId is required",
				})
				continue
			}
			newMode := agent.SessionMode(msg.Content)
			if newMode != agent.ModePlan && newMode != agent.ModeImplement {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "Invalid mode: " + msg.Content + " (must be 'plan' or 'implement')",
				})
				continue
			}
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				continue
			}
			adapter.SetSessionMode(msg.SessionID, newMode)
			writeJSON(chatOutbound{
				Type:      "event",
				SessionID: msg.SessionID,
				Event: &agent.Event{
					Type:      agent.EventSessionUpdated,
					SessionID: msg.SessionID,
					Content:   string(newMode),
					Meta: map[string]interface{}{
						"mode": string(newMode),
					},
				},
			})

		case "set_agent":
			if msg.SessionID == "" {
				writeJSON(chatOutbound{
					Type:    "error",
					Content: "sessionId is required",
				})
				continue
			}
			if msg.Content == "" {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "agent name is required",
				})
				continue
			}
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				continue
			}
			adapter.SetSessionAgent(msg.SessionID, msg.Content)
			writeJSON(chatOutbound{
				Type:      "event",
				SessionID: msg.SessionID,
				Event: &agent.Event{
					Type:      agent.EventSessionUpdated,
					SessionID: msg.SessionID,
					Content:   msg.Content,
					Meta: map[string]interface{}{
						"agent": msg.Content,
					},
				},
			})

		case "set_model":
			if msg.SessionID == "" {
				writeJSON(chatOutbound{
					Type:    "error",
					Content: "sessionId is required",
				})
				continue
			}
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				continue
			}
			adapter.SetSessionModel(msg.SessionID, msg.Content)
			writeJSON(chatOutbound{
				Type:      "event",
				SessionID: msg.SessionID,
				Event: &agent.Event{
					Type:      agent.EventSessionUpdated,
					SessionID: msg.SessionID,
					Content:   msg.Content,
					Meta: map[string]interface{}{
						"model": msg.Content,
					},
				},
			})

		case "permission_response":
			if msg.SessionID == "" {
				writeJSON(chatOutbound{
					Type:    "error",
					Content: "sessionId is required",
				})
				continue
			}
			if msg.PermissionID == "" {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "permissionId is required",
				})
				continue
			}
			if msg.Response != "once" && msg.Response != "always" && msg.Response != "reject" {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "Invalid response: " + msg.Response + " (must be 'once', 'always', or 'reject')",
				})
				continue
			}
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "No agent instance for session: " + err.Error(),
				})
				continue
			}
			if err := adapter.RespondToPermission(ctx, msg.SessionID, msg.PermissionID, msg.Response, msg.Remember); err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "Failed to respond to permission: " + err.Error(),
				})
			}

		case "question_response":
			if msg.SessionID == "" {
				writeJSON(chatOutbound{
					Type:    "error",
					Content: "sessionId is required",
				})
				continue
			}
			if msg.QuestionID == "" {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "questionId is required",
				})
				continue
			}
			if len(msg.Answers) == 0 {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "answers are required",
				})
				continue
			}
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "No agent instance for session: " + err.Error(),
				})
				continue
			}
			if err := adapter.RespondToQuestion(ctx, msg.QuestionID, msg.Answers); err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "Failed to respond to question: " + err.Error(),
				})
			}

		case "question_reject":
			if msg.SessionID == "" {
				writeJSON(chatOutbound{
					Type:    "error",
					Content: "sessionId is required",
				})
				continue
			}
			if msg.QuestionID == "" {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "questionId is required",
				})
				continue
			}
			adapter, err := s.resolveAdapter(r, msg.SessionID)
			if err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "No agent instance for session: " + err.Error(),
				})
				continue
			}
			if err := adapter.RejectQuestion(ctx, msg.QuestionID); err != nil {
				writeJSON(chatOutbound{
					Type:      "error",
					SessionID: msg.SessionID,
					Content:   "Failed to reject question: " + err.Error(),
				})
			}

		default:
			writeJSON(chatOutbound{
				Type:    "error",
				Content: "Unknown message type: " + msg.Type,
			})
		}
	}
}

// handleWSVoice handles WebSocket connections for the voice pipeline.
// Audio flows up (client → server for STT), text/audio flows down (TTS → client).
func (s *Server) handleWSVoice(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
	}
	defer conn.Close()

	// TODO: Implement voice pipeline
	// 1. Receive audio chunks from client (push-to-talk)
	// 2. Send to STT provider for transcription
	// 3. Route transcribed text (app command or agent message)
	// 4. Stream agent response through TTS filter
	// 5. Synthesize filtered text via TTS provider
	// 6. Send audio chunks back to client

	slog.Info("voice WebSocket connected, pipeline not yet implemented")

	conn.WriteJSON(map[string]string{
		"type":    "error",
		"content": "Voice pipeline not yet implemented",
	})

	// Keep connection alive until closed
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
	}
}
