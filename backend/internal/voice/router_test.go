package voice

import "testing"

func TestRoute_AppCommands(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd AppCommand
	}{
		{"new session", AppCommandNewSession},
		{"Create session please", AppCommandNewSession},
		{"let's start new", AppCommandNewSession},
		{"stop", AppCommandStop},
		{"cancel", AppCommandStop},
		{"shut up", AppCommandStop},
		{"read that again", AppCommandRepeat},
		{"repeat that", AppCommandRepeat},
		{"say that again", AppCommandRepeat},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Route(tt.input)
			if r.Type != CommandApp {
				t.Fatalf("expected CommandApp, got CommandAgent for %q", tt.input)
			}
			if r.AppCommand != tt.wantCmd {
				t.Errorf("expected %q, got %q", tt.wantCmd, r.AppCommand)
			}
		})
	}
}

func TestRoute_AgentMessages(t *testing.T) {
	inputs := []string{
		"what does this function do",
		"refactor the handler",
		"explain the error",
		"",
		"   ",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			r := Route(input)
			if r.Type != CommandAgent {
				t.Fatalf("expected CommandAgent for %q, got CommandApp(%s)", input, r.AppCommand)
			}
		})
	}
}
