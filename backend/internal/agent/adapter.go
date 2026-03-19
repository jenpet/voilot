package agent

import "context"

// SessionMode controls what the agent is allowed to do.
type SessionMode string

const (
	// ModePlan restricts the agent to discussion and planning only.
	ModePlan SessionMode = "plan"
	// ModeImplement gives the agent full tool access for code changes.
	ModeImplement SessionMode = "implement"
)

// Session represents an active conversation with an agent.
// Aligned with OpenCode's Session type.
type Session struct {
	ID        string      `json:"id"`
	Title     string      `json:"title,omitempty"`
	Mode      SessionMode `json:"mode,omitempty"`
	ProjectID string      `json:"projectId,omitempty"`
	Time      *TimeInfo   `json:"time,omitempty"`
}

// TimeInfo tracks creation/update timestamps.
type TimeInfo struct {
	Created int64 `json:"created,omitempty"`
	Updated int64 `json:"updated,omitempty"`
}

// SessionOptions configures a new session.
type SessionOptions struct {
	Title    string      `json:"title,omitempty"`
	Mode     SessionMode `json:"mode,omitempty"`
	ParentID string      `json:"parentID,omitempty"`
}

// MessagePart represents one part of a message to send.
type MessagePart struct {
	Type string `json:"type"` // "text", "file"
	Text string `json:"text,omitempty"`
}

// Status reports the health and identity of an agent backend.
type Status struct {
	Connected bool   `json:"connected"`
	Provider  string `json:"provider"` // e.g. "opencode", "claude"
	Model     string `json:"model,omitempty"`
	Version   string `json:"version,omitempty"`
}

// HistoryMessage is a simplified message for displaying session history.
// It combines data from the OpenCode message info and parts into a flat structure.
type HistoryMessage struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"` // "user" or "assistant"
	Content   string                 `json:"content"`
	Timestamp int64                  `json:"timestamp"`
	Type      string                 `json:"type,omitempty"` // "text", "tool_use", etc.
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// Adapter defines the interface that all agent backends must implement.
// The first implementation will be OpenCode; Claude Agent SDK comes later.
type Adapter interface {
	// CreateSession starts a new conversation session.
	CreateSession(ctx context.Context, opts SessionOptions) (*Session, error)

	// ResumeSession reconnects to an existing session by ID.
	ResumeSession(ctx context.Context, id string) (*Session, error)

	// ListSessions returns all available sessions.
	ListSessions(ctx context.Context) ([]Session, error)

	// DeleteSession removes a session.
	DeleteSession(ctx context.Context, id string) error

	// GetMessages retrieves the message history for a session.
	GetMessages(ctx context.Context, sessionID string) ([]HistoryMessage, error)

	// SendMessage sends a user message and returns a channel of streaming events.
	// The channel is closed when the response is complete.
	SendMessage(ctx context.Context, sessionID string, message string) (<-chan Event, error)

	// SendMessageAsync sends a message without waiting for a response.
	// Events arrive via the SSE stream.
	// If mode is ModePlan, the "planitect" agent is selected in OpenCode.
	SendMessageAsync(ctx context.Context, sessionID string, message string, mode SessionMode) error

	// AbortSession aborts a running session.
	AbortSession(ctx context.Context, sessionID string) error

	// SetSessionMode sets the mode for a session (plan or implement).
	// Mode is a voilot-level concept and is not forwarded to the agent backend.
	SetSessionMode(sessionID string, mode SessionMode)

	// GetSessionMode returns the current mode for a session.
	// Returns ModePlan if the session has no mode set.
	GetSessionMode(sessionID string) SessionMode

	// GetStatus returns the current connection status of the agent backend.
	GetStatus(ctx context.Context) (*Status, error)

	// SubscribeEvents returns a channel that receives all SSE events from the agent.
	// The channel is closed when the context is cancelled or the connection drops.
	SubscribeEvents(ctx context.Context) (<-chan Event, error)
}
