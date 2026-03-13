package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/jenpet/voilot/internal/voice"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins in development. In production, nginx handles CORS.
		return true
	},
}

// chatMessage is the WebSocket message format for the chat endpoint.
type chatMessage struct {
	Type      string `json:"type"`      // "message", "command", "event"
	SessionID string `json:"sessionId"` // target session
	Content   string `json:"content"`   // message text or command
}

// handleWSChat handles WebSocket connections for real-time text chat.
func (s *Server) handleWSChat(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	for {
		var msg chatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		switch msg.Type {
		case "message":
			// Route through command router first
			result := voice.Route(msg.Content)
			if result.Type == voice.CommandApp {
				// Handle app command and send response
				resp := chatMessage{
					Type:    "command",
					Content: string(result.AppCommand),
				}
				conn.WriteJSON(resp)
				continue
			}

			// Forward to agent
			events, err := s.agentAdapter.SendMessage(r.Context(), msg.SessionID, result.Text)
			if err != nil {
				conn.WriteJSON(chatMessage{
					Type:    "event",
					Content: "error: " + err.Error(),
				})
				continue
			}

			// Stream agent events to the WebSocket
			for event := range events {
				data, _ := json.Marshal(event)
				conn.WriteJSON(chatMessage{
					Type:    "event",
					Content: string(data),
				})
			}
		}
	}
}

// handleWSVoice handles WebSocket connections for the voice pipeline.
// Audio flows up (client → server for STT), text/audio flows down (TTS → client).
func (s *Server) handleWSVoice(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// TODO: Implement voice pipeline
	// 1. Receive audio chunks from client (push-to-talk)
	// 2. Send to STT provider for transcription
	// 3. Route transcribed text (app command or agent message)
	// 4. Stream agent response through TTS filter
	// 5. Synthesize filtered text via TTS provider
	// 6. Send audio chunks back to client

	log.Println("Voice WebSocket connected — pipeline not yet implemented")

	// Keep connection alive until closed
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
	}
}
