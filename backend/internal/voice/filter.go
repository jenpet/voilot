package voice

import (
	"fmt"
	"strings"

	"github.com/jenpet/voilot/internal/agent"
)

// FilterResult holds the processed text ready for TTS.
type FilterResult struct {
	// TextForTTS is the text that should be spoken aloud.
	TextForTTS string
	// ShouldSpeak indicates whether TTS should be triggered at all.
	ShouldSpeak bool
}

// FilterForTTS takes an agent event and decides what (if anything) should be
// spoken aloud. Code blocks get summarized, thinking is skipped, errors are
// announced, and text is passed through.
func FilterForTTS(event agent.Event) FilterResult {
	switch event.Type {
	case agent.EventText:
		text := strings.TrimSpace(event.Content)
		if text == "" {
			return FilterResult{ShouldSpeak: false}
		}
		return FilterResult{TextForTTS: text, ShouldSpeak: true}

	case agent.EventCode:
		// Summarize code blocks instead of reading them verbatim.
		lang := event.Language
		if lang == "" {
			lang = "code"
		}
		lines := strings.Count(event.Content, "\n") + 1
		summary := fmt.Sprintf("Wrote %d lines of %s.", lines, lang)
		return FilterResult{TextForTTS: summary, ShouldSpeak: true}

	case agent.EventToolUse:
		// Briefly announce tool usage.
		toolName := ""
		if name, ok := event.Meta["tool"].(string); ok {
			toolName = name
		}
		if toolName != "" {
			return FilterResult{
				TextForTTS:  fmt.Sprintf("Using %s.", toolName),
				ShouldSpeak: true,
			}
		}
		return FilterResult{ShouldSpeak: false}

	case agent.EventThinking:
		// Skip thinking blocks — they're internal reasoning.
		return FilterResult{ShouldSpeak: false}

	case agent.EventError:
		return FilterResult{
			TextForTTS:  fmt.Sprintf("Error: %s", event.Content),
			ShouldSpeak: true,
		}

	case agent.EventDone:
		// Optionally announce completion.
		if event.Content != "" {
			return FilterResult{TextForTTS: event.Content, ShouldSpeak: true}
		}
		return FilterResult{ShouldSpeak: false}

	default:
		return FilterResult{ShouldSpeak: false}
	}
}
