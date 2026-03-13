package stt

import (
	"context"
	"fmt"
)

// WhisperProvider connects to a faster-whisper server via HTTP.
type WhisperProvider struct {
	baseURL string
}

// NewWhisperProvider creates a new faster-whisper client.
func NewWhisperProvider(baseURL string) *WhisperProvider {
	return &WhisperProvider{baseURL: baseURL}
}

func (p *WhisperProvider) Transcribe(ctx context.Context, req Request) (*Result, error) {
	// TODO: POST audio to faster-whisper HTTP API, return transcription
	return nil, fmt.Errorf("not implemented")
}

func (p *WhisperProvider) Name() string {
	return "whisper"
}

// Verify interface compliance at compile time.
var _ Provider = (*WhisperProvider)(nil)
