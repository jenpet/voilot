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
type Session struct {
	ID    string      `json:"id"`
	Title string      `json:"title,omitempty"`
	Mode  SessionMode `json:"mode"`
}

// SessionOptions configures a new session.
type SessionOptions struct {
	Title string      `json:"title,omitempty"`
	Mode  SessionMode `json:"mode"`
}

// Status reports the health and identity of an agent backend.
type Status struct {
	Connected bool   `json:"connected"`
	Provider  string `json:"provider"` // e.g. "opencode", "claude"
	Model     string `json:"model,omitempty"`
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

	// SendMessage sends a user message and returns a channel of streaming events.
	// The channel is closed when the response is complete.
	SendMessage(ctx context.Context, sessionID string, message string) (<-chan Event, error)

	// GetStatus returns the current connection status of the agent backend.
	GetStatus(ctx context.Context) (*Status, error)
}
