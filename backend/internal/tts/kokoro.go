package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// KokoroProvider connects to a Kokoro-FastAPI server via the OpenAI-compatible API.
type KokoroProvider struct {
	baseURL string
	client  *http.Client
}

// NewKokoroProvider creates a new Kokoro TTS client.
func NewKokoroProvider(baseURL string) *KokoroProvider {
	return &KokoroProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// DefaultKokoroVoice is the default voice for Kokoro TTS.
const DefaultKokoroVoice = "af_heart"

// kokoroSpeechRequest is the OpenAI-compatible request body.
type kokoroSpeechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed,omitempty"`
}

// kokoroVoicesResponse is the response from GET /v1/audio/voices.
type kokoroVoicesResponse struct {
	Voices []string `json:"voices"`
}

func (p *KokoroProvider) Synthesize(ctx context.Context, req Request) (*Response, error) {
	voice := req.Voice
	if voice == "" {
		voice = DefaultKokoroVoice
	}

	kokoroReq := kokoroSpeechRequest{
		Model:          "kokoro",
		Input:          req.Text,
		Voice:          voice,
		ResponseFormat: "wav",
	}
	if req.Speed > 0 {
		kokoroReq.Speed = req.Speed
	}

	body, err := json.Marshal(kokoroReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kokoro TTS request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kokoro TTS returned status %d: %s", resp.StatusCode, string(respBody))
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/wav"
	}

	return &Response{
		Audio:       resp.Body,
		ContentType: contentType,
	}, nil
}

func (p *KokoroProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/audio/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kokoro voices request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kokoro voices returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var kokoroResp kokoroVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&kokoroResp); err != nil {
		return nil, fmt.Errorf("failed to decode voices response: %w", err)
	}

	voices := make([]Voice, 0, len(kokoroResp.Voices))
	for _, v := range kokoroResp.Voices {
		voices = append(voices, Voice{
			ID:   v,
			Name: v,
		})
	}

	return voices, nil
}

func (p *KokoroProvider) Name() string {
	return "kokoro"
}

// Verify interface compliance at compile time.
var _ Provider = (*KokoroProvider)(nil)
