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
	ID            string      `json:"id"`
	Title         string      `json:"title,omitempty"`
	Mode          SessionMode `json:"mode,omitempty"`
	Agent         string      `json:"agent,omitempty"`         // active agent name (e.g. "build", "planitect")
	Model         string      `json:"model,omitempty"`         // active model override (provider/model)
	LastUsedModel string      `json:"lastUsedModel,omitempty"` // last model used in this session (provider/model)
	ProjectID     string      `json:"projectId,omitempty"`
	Time          *TimeInfo   `json:"time,omitempty"`
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
	Agent    string      `json:"agent,omitempty"` // agent to use (e.g. "build", "planitect")
	Model    string      `json:"model,omitempty"` // model override (provider/model)
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

// AgentInfo describes an available agent that can handle messages.
type AgentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"` // hex color for UI display
}

// ModelInfo describes an available model that can be selected for a session.
type ModelInfo struct {
	ID           string `json:"id"`                     // provider/model
	Name         string `json:"name,omitempty"`         // human-friendly model name
	ProviderID   string `json:"providerId,omitempty"`   // provider key
	ProviderName string `json:"providerName,omitempty"` // provider display name
}

// ModelCatalog describes selectable models and the OpenCode default model.
type ModelCatalog struct {
	Models       []ModelInfo `json:"models"`
	DefaultModel string      `json:"defaultModel,omitempty"`
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
	// The agent parameter specifies which agent to use (e.g. "build", "planitect").
	// The model parameter is an optional per-session override in provider/model format.
	// Empty values use the agent backend's defaults.
	SendMessageAsync(ctx context.Context, sessionID string, message string, agentName string, modelID string) error

	// AbortSession aborts a running session.
	AbortSession(ctx context.Context, sessionID string) error

	// ListAgents returns the available agents from the backend.
	// Only returns user-facing agents (filters out hidden/internal ones).
	ListAgents(ctx context.Context) ([]AgentInfo, error)

	// SetSessionAgent sets the active agent for a session.
	SetSessionAgent(sessionID string, agentName string)

	// GetSessionAgent returns the active agent for a session.
	// Returns "planitect" if no agent has been set.
	GetSessionAgent(sessionID string) string

	// ListModels returns selectable models and the OpenCode default model.
	ListModels(ctx context.Context) (*ModelCatalog, error)

	// SetSessionModel sets the active model override for a session.
	// Empty modelID clears the override and falls back to OpenCode default.
	SetSessionModel(sessionID string, modelID string)

	// GetSessionModel returns the model override for a session.
	// Returns empty string when no override is set.
	GetSessionModel(sessionID string) string

	// SetSessionMode sets the mode for a session (plan or implement).
	// Mode is a voilot-level concept and is not forwarded to the agent backend.
	SetSessionMode(sessionID string, mode SessionMode)

	// GetSessionMode returns the current mode for a session.
	// Returns ModePlan if the session has no mode set.
	GetSessionMode(sessionID string) SessionMode

	// GetSessionBusy returns true if the session is currently busy (agent is processing).
	// Used by the frontend on page reload / WebSocket reconnect to detect stale state.
	GetSessionBusy(sessionID string) bool

	// GetStatus returns the current connection status of the agent backend.
	GetStatus(ctx context.Context) (*Status, error)

	// SubscribeEvents returns a channel that receives all SSE events from the agent.
	// The channel is closed when the context is cancelled or the connection drops.
	SubscribeEvents(ctx context.Context) (<-chan Event, error)

	// RespondToPermission sends a response to a pending permission prompt.
	// response must be "once", "always", or "reject".
	// If remember is true, the rule is persisted for the session.
	RespondToPermission(ctx context.Context, sessionID, permissionID, response string, remember bool) error

	// RespondToQuestion sends answers to a pending question prompt.
	// answers is a parallel array: one inner array per question, each containing selected labels.
	RespondToQuestion(ctx context.Context, requestID string, answers [][]string) error

	// RejectQuestion dismisses a pending question prompt without answering.
	RejectQuestion(ctx context.Context, requestID string) error
}
