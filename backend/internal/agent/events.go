package agent

import "encoding/json"

// EventType classifies the kind of content in an agent response stream.
type EventType string

const (
	// EventText is a natural language text chunk intended for TTS.
	EventText EventType = "text"
	// EventCode is a code block that should be summarized, not spoken verbatim.
	EventCode EventType = "code"
	// EventToolUse indicates the agent is invoking a tool.
	EventToolUse EventType = "tool_use"
	// EventToolResult is the result of a tool invocation.
	EventToolResult EventType = "tool_result"
	// EventThinking is internal reasoning that can be skipped or summarized.
	EventThinking EventType = "thinking"
	// EventError signals an error in the agent response.
	EventError EventType = "error"
	// EventDone marks the end of the response stream.
	EventDone EventType = "done"
	// EventStatus reports session status changes (idle, busy, retry).
	EventStatus EventType = "status"
	// EventSessionCreated signals a new session was created.
	EventSessionCreated EventType = "session_created"
	// EventSessionUpdated signals a session was updated (title, etc).
	EventSessionUpdated EventType = "session_updated"
)

// Event represents a single streaming event from an agent.
type Event struct {
	Type      EventType              `json:"type"`
	SessionID string                 `json:"sessionId,omitempty"`
	MessageID string                 `json:"messageId,omitempty"`
	PartID    string                 `json:"partId,omitempty"`
	Content   string                 `json:"content"`
	Language  string                 `json:"language,omitempty"` // for code blocks
	Delta     string                 `json:"delta,omitempty"`    // incremental text update
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// OpenCodeSSEEvent represents a raw SSE event from the OpenCode /event stream.
type OpenCodeSSEEvent struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

// OpenCodePartUpdate is the properties payload for "message.part.updated".
type OpenCodePartUpdate struct {
	Part  json.RawMessage `json:"part"`
	Delta string          `json:"delta,omitempty"`
}

// OpenCodePartDelta is the properties payload for "message.part.delta".
type OpenCodePartDelta struct {
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	PartID    string `json:"partID"`
	Field     string `json:"field"` // "text", etc.
	Delta     string `json:"delta"`
}

// OpenCodePart is a generic part from OpenCode.
type OpenCodePart struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionID"`
	MessageID string          `json:"messageID"`
	Type      string          `json:"type"` // "text", "reasoning", "tool", "step-start", "step-finish", etc.
	Text      string          `json:"text,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	CallID    string          `json:"callID,omitempty"`
	State     json.RawMessage `json:"state,omitempty"`
}

// OpenCodeToolState is the state field inside a tool part.
type OpenCodeToolState struct {
	Status   string                 `json:"status"` // "pending", "running", "completed", "error"
	Input    map[string]interface{} `json:"input,omitempty"`
	Output   string                 `json:"output,omitempty"`
	Title    string                 `json:"title,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// OpenCodeSessionStatus is the properties payload for "session.status".
type OpenCodeSessionStatus struct {
	SessionID string `json:"sessionID"`
	Status    struct {
		Type    string `json:"type"` // "idle", "busy", "retry"
		Message string `json:"message,omitempty"`
	} `json:"status"`
}

// OpenCodeSessionInfo wraps session info in event properties.
type OpenCodeSessionInfo struct {
	Info json.RawMessage `json:"info"`
}

// OpenCodeMessageInfo wraps message info in event properties.
type OpenCodeMessageInfo struct {
	Info json.RawMessage `json:"info"`
}

// OpenCodeSessionError is the properties payload for "session.error".
type OpenCodeSessionError struct {
	SessionID string          `json:"sessionID,omitempty"`
	Error     json.RawMessage `json:"error,omitempty"`
}
