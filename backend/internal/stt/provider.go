package stt

import (
	"context"
	"io"
)

// Request contains audio data to be transcribed.
type Request struct {
	// Audio is the raw audio data (WAV, WebM, etc.).
	Audio io.Reader
	// ContentType is the MIME type of the audio (e.g. "audio/wav", "audio/webm").
	ContentType string
	// Language is an optional language hint (e.g. "en", "de").
	Language string
}

// Result is the transcription output.
type Result struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence,omitempty"`
	Language   string  `json:"language,omitempty"` // detected language
}

// Provider defines the interface for speech-to-text backends.
// First implementation: faster-whisper via HTTP.
type Provider interface {
	// Transcribe converts audio to text.
	Transcribe(ctx context.Context, req Request) (*Result, error)

	// Name returns the provider identifier (e.g. "whisper", "web-speech").
	Name() string
}
