package agent

import (
	"context"
	"fmt"
)

// OpenCodeAdapter connects to an OpenCode server via its HTTP API and SSE events.
type OpenCodeAdapter struct {
	baseURL string
}

// NewOpenCodeAdapter creates a new adapter pointing at the given OpenCode server URL.
func NewOpenCodeAdapter(baseURL string) *OpenCodeAdapter {
	return &OpenCodeAdapter{baseURL: baseURL}
}

func (a *OpenCodeAdapter) CreateSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	// TODO: POST /session to OpenCode API
	return nil, fmt.Errorf("not implemented")
}

func (a *OpenCodeAdapter) ResumeSession(ctx context.Context, id string) (*Session, error) {
	// TODO: GET /session/:id from OpenCode API
	return nil, fmt.Errorf("not implemented")
}

func (a *OpenCodeAdapter) ListSessions(ctx context.Context) ([]Session, error) {
	// TODO: GET /session from OpenCode API
	return nil, fmt.Errorf("not implemented")
}

func (a *OpenCodeAdapter) DeleteSession(ctx context.Context, id string) error {
	// TODO: DELETE /session/:id on OpenCode API
	return fmt.Errorf("not implemented")
}

func (a *OpenCodeAdapter) SendMessage(ctx context.Context, sessionID string, message string) (<-chan Event, error) {
	// TODO: POST /session/:id/message and stream SSE events
	return nil, fmt.Errorf("not implemented")
}

func (a *OpenCodeAdapter) GetStatus(ctx context.Context) (*Status, error) {
	// TODO: health check against OpenCode API
	return &Status{
		Connected: false,
		Provider:  "opencode",
	}, nil
}

// Verify interface compliance at compile time.
var _ Adapter = (*OpenCodeAdapter)(nil)
