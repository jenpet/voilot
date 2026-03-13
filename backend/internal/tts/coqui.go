package tts

import (
	"context"
	"fmt"
)

// CoquiProvider connects to a Coqui TTS server via HTTP.
type CoquiProvider struct {
	baseURL string
}

// NewCoquiProvider creates a new Coqui TTS client.
func NewCoquiProvider(baseURL string) *CoquiProvider {
	return &CoquiProvider{baseURL: baseURL}
}

func (p *CoquiProvider) Synthesize(ctx context.Context, req Request) (*Response, error) {
	// TODO: POST to Coqui TTS HTTP API with text, return audio stream
	return nil, fmt.Errorf("not implemented")
}

func (p *CoquiProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	// TODO: query available voices from Coqui API
	return nil, fmt.Errorf("not implemented")
}

func (p *CoquiProvider) Name() string {
	return "coqui"
}

// Verify interface compliance at compile time.
var _ Provider = (*CoquiProvider)(nil)
