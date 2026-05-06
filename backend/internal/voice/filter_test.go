package voice

import (
	"testing"

	"github.com/jenpet/voilot/internal/agent"
)

func TestFilterForTTS_Text(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventText, Content: "Hello world"})
	if !r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=true for text")
	}
	if r.TextForTTS != "Hello world" {
		t.Errorf("expected %q, got %q", "Hello world", r.TextForTTS)
	}
}

func TestFilterForTTS_EmptyText(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventText, Content: "  "})
	if r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=false for empty text")
	}
}

func TestFilterForTTS_Code(t *testing.T) {
	code := "func main() {\n\tfmt.Println(\"hello\")\n}"
	r := FilterForTTS(agent.Event{Type: agent.EventCode, Content: code, Language: "Go"})
	if !r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=true for code")
	}
	if r.TextForTTS != "Wrote 3 lines of Go." {
		t.Errorf("expected %q, got %q", "Wrote 3 lines of Go.", r.TextForTTS)
	}
}

func TestFilterForTTS_CodeNoLanguage(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventCode, Content: "x = 1"})
	if r.TextForTTS != "Wrote 1 lines of code." {
		t.Errorf("expected %q, got %q", "Wrote 1 lines of code.", r.TextForTTS)
	}
}

func TestFilterForTTS_Thinking(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventThinking, Content: "let me think..."})
	if r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=false for thinking")
	}
}

func TestFilterForTTS_Error(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventError, Content: "something broke"})
	if !r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=true for error")
	}
	if r.TextForTTS != "Error: something broke" {
		t.Errorf("expected %q, got %q", "Error: something broke", r.TextForTTS)
	}
}

func TestFilterForTTS_ToolUse(t *testing.T) {
	r := FilterForTTS(agent.Event{
		Type: agent.EventToolUse,
		Meta: map[string]interface{}{"tool": "bash"},
	})
	if !r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=true for tool use with name")
	}
	if r.TextForTTS != "Using bash." {
		t.Errorf("expected %q, got %q", "Using bash.", r.TextForTTS)
	}
}

func TestFilterForTTS_ToolUseNoName(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventToolUse})
	if r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=false for tool use without name")
	}
}

func TestFilterForTTS_Done(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventDone, Content: ""})
	if r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=false for empty done")
	}

	r = FilterForTTS(agent.Event{Type: agent.EventDone, Content: "Task complete"})
	if !r.ShouldSpeak || r.TextForTTS != "Task complete" {
		t.Errorf("expected spoken 'Task complete', got speak=%v text=%q", r.ShouldSpeak, r.TextForTTS)
	}
}

func TestFilterForTTS_Question(t *testing.T) {
	r := FilterForTTS(agent.Event{Type: agent.EventQuestionRequest, Content: "Which option?"})
	if !r.ShouldSpeak {
		t.Fatal("expected ShouldSpeak=true for question")
	}
	if r.TextForTTS != "Question: Which option?" {
		t.Errorf("expected %q, got %q", "Question: Which option?", r.TextForTTS)
	}
}
