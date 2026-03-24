package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// WhisperProvider connects to a faster-whisper server via HTTP.
type WhisperProvider struct {
	baseURL string
	client  *http.Client
}

// NewWhisperProvider creates a new faster-whisper client.
func NewWhisperProvider(baseURL string) *WhisperProvider {
	return &WhisperProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 60 * time.Second, // transcription can take a while
		},
	}
}

// whisperResponse is the JSON response from the faster-whisper sidecar.
type whisperResponse struct {
	Text                string           `json:"text"`
	Segments            []whisperSegment `json:"segments,omitempty"`
	Language            string           `json:"language,omitempty"`
	LanguageProbability float64          `json:"language_probability,omitempty"`
	Duration            float64          `json:"duration,omitempty"`
}

type whisperSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func (p *WhisperProvider) Transcribe(ctx context.Context, req Request) (*Result, error) {
	// Read all audio data into memory to build multipart form
	audioData, err := io.ReadAll(req.Audio)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %w", err)
	}

	// Build multipart form body (the sidecar expects 'audio' file field)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Determine file extension from content type
	ext := extensionForContentType(req.ContentType)
	part, err := writer.CreateFormFile("audio", "recording"+ext)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return nil, fmt.Errorf("failed to write audio data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Build URL with optional language hint
	url := p.baseURL + "/transcribe"
	if req.Language != "" {
		url += "?language=" + req.Language
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("faster-whisper request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("faster-whisper returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var whisperResp whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&whisperResp); err != nil {
		return nil, fmt.Errorf("failed to decode whisper response: %w", err)
	}

	return &Result{
		Text:       whisperResp.Text,
		Confidence: whisperResp.LanguageProbability,
		Language:   whisperResp.Language,
	}, nil
}

func (p *WhisperProvider) Name() string {
	return "whisper"
}

// HealthCheck pings the faster-whisper /health endpoint.
func (p *WhisperProvider) HealthCheck(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// extensionForContentType maps common audio MIME types to file extensions.
func extensionForContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "wav"):
		return ".wav"
	case strings.Contains(ct, "webm"):
		return ".webm"
	case strings.Contains(ct, "ogg"):
		return ".ogg"
	case strings.Contains(ct, "mp3") || strings.Contains(ct, "mpeg"):
		return ".mp3"
	case strings.Contains(ct, "flac"):
		return ".flac"
	case strings.Contains(ct, "mp4") || strings.Contains(ct, "m4a"):
		return ".m4a"
	default:
		return ".wav"
	}
}

// Verify interface compliance at compile time.
var _ Provider = (*WhisperProvider)(nil)
