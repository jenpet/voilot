package tts

import (
	"context"
	"io"
)

// Request describes what text should be synthesized.
type Request struct {
	Text     string  `json:"text"`
	Voice    string  `json:"voice,omitempty"`    // voice ID or name
	Language string  `json:"language,omitempty"` // e.g. "en", "de"
	Speed    float64 `json:"speed,omitempty"`    // speech rate multiplier
}

// Response contains the synthesized audio.
type Response struct {
	// Audio is the raw audio data (WAV or MP3).
	Audio io.ReadCloser
	// ContentType is the MIME type of the audio (e.g. "audio/wav").
	ContentType string
}

// Provider defines the interface for text-to-speech backends.
// First implementation: Coqui XTTSv2 via HTTP.
type Provider interface {
	// Synthesize converts text to speech audio.
	Synthesize(ctx context.Context, req Request) (*Response, error)

	// ListVoices returns available voice options.
	ListVoices(ctx context.Context) ([]Voice, error)

	// Name returns the provider identifier (e.g. "coqui", "openai").
	Name() string
}

// Voice describes an available TTS voice.
type Voice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language,omitempty"`
}
