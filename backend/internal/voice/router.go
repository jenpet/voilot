package voice

import "strings"

// CommandType classifies a voice input as either an app control command
// or a message to be forwarded to the agent.
type CommandType int

const (
	// CommandAgent means the input should be sent to the agent as a message.
	CommandAgent CommandType = iota
	// CommandApp means the input is an application control command.
	CommandApp
)

// AppCommand represents a recognized application-level voice command.
type AppCommand string

const (
	AppCommandNewSession AppCommand = "new_session"
	AppCommandStop       AppCommand = "stop"
	AppCommandRepeat     AppCommand = "repeat"
)

// RouteResult is the output of the command router.
type RouteResult struct {
	Type       CommandType
	AppCommand AppCommand // set when Type == CommandApp
	Text       string     // original or cleaned text
}

// Route analyzes transcribed text and determines whether it's an app command
// or a message for the agent.
func Route(text string) RouteResult {
	lower := strings.TrimSpace(strings.ToLower(text))

	// App control commands — simple keyword matching.
	// This can be made more sophisticated later (e.g. fuzzy matching, NLU).
	switch {
	case contains(lower, "new session", "create session", "start new"):
		return RouteResult{Type: CommandApp, AppCommand: AppCommandNewSession, Text: text}
	case lower == "stop" || lower == "cancel" || lower == "shut up":
		return RouteResult{Type: CommandApp, AppCommand: AppCommandStop, Text: text}
	case contains(lower, "read that again", "repeat that", "say that again"):
		return RouteResult{Type: CommandApp, AppCommand: AppCommandRepeat, Text: text}
	default:
		return RouteResult{Type: CommandAgent, Text: text}
	}
}

// contains checks if s contains any of the given substrings.
func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
