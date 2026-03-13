package agent

// EventType classifies the kind of content in an agent response stream.
type EventType string

const (
	// EventText is a natural language text chunk intended for TTS.
	EventText EventType = "text"
	// EventCode is a code block that should be summarized, not spoken verbatim.
	EventCode EventType = "code"
	// EventToolUse indicates the agent is invoking a tool.
	EventToolUse EventType = "tool_use"
	// EventThinking is internal reasoning that can be skipped or summarized.
	EventThinking EventType = "thinking"
	// EventError signals an error in the agent response.
	EventError EventType = "error"
	// EventDone marks the end of the response stream.
	EventDone EventType = "done"
)

// Event represents a single streaming event from an agent.
type Event struct {
	Type     EventType              `json:"type"`
	Content  string                 `json:"content"`
	Language string                 `json:"language,omitempty"` // for code blocks
	Meta     map[string]interface{} `json:"meta,omitempty"`
}
