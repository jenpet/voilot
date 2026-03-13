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

// CoquiProvider connects to a Coqui TTS server via HTTP.
type CoquiProvider struct {
	baseURL string
	client  *http.Client
}

// NewCoquiProvider creates a new Coqui TTS client.
func NewCoquiProvider(baseURL string) *CoquiProvider {
	return &CoquiProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 60 * time.Second, // synthesis can take a while for long text
		},
	}
}

// coquiSynthRequest is the JSON body sent to the Coqui sidecar.
type coquiSynthRequest struct {
	Text       string `json:"text"`
	Language   string `json:"language,omitempty"`
	Speaker    string `json:"speaker,omitempty"`
	SpeakerWAV string `json:"speaker_wav,omitempty"`
}

// coquiVoicesResponse is the JSON response from the Coqui /voices endpoint.
type coquiVoicesResponse struct {
	Speakers  []string `json:"speakers"`
	Languages []string `json:"languages"`
}

func (p *CoquiProvider) Synthesize(ctx context.Context, req Request) (*Response, error) {
	coquiReq := coquiSynthRequest{
		Text:     req.Text,
		Language: req.Language,
		Speaker:  req.Voice,
	}
	if coquiReq.Language == "" {
		coquiReq.Language = "en"
	}

	body, err := json.Marshal(coquiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/synthesize", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("coqui TTS request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("coqui TTS returned status %d: %s", resp.StatusCode, string(respBody))
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/wav"
	}

	return &Response{
		Audio:       resp.Body, // caller is responsible for closing
		ContentType: contentType,
	}, nil
}

func (p *CoquiProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("coqui voices request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("coqui voices returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var coquiResp coquiVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&coquiResp); err != nil {
		return nil, fmt.Errorf("failed to decode voices response: %w", err)
	}

	voices := make([]Voice, 0, len(coquiResp.Speakers))
	for _, speaker := range coquiResp.Speakers {
		voices = append(voices, Voice{
			ID:   speaker,
			Name: speaker,
		})
	}

	return voices, nil
}

func (p *CoquiProvider) Name() string {
	return "coqui"
}

// Verify interface compliance at compile time.
var _ Provider = (*CoquiProvider)(nil)
