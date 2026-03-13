package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OpenCodeAdapter connects to an OpenCode server via its HTTP API and SSE events.
type OpenCodeAdapter struct {
	baseURL    string
	httpClient *http.Client

	// SSE event fan-out: multiple subscribers can listen to the event stream.
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	sseRunning  bool
}

// NewOpenCodeAdapter creates a new adapter pointing at the given OpenCode server URL.
func NewOpenCodeAdapter(baseURL string) *OpenCodeAdapter {
	return &OpenCodeAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		subscribers: make(map[chan Event]struct{}),
	}
}

// doRequest executes an HTTP request against the OpenCode server.
func (a *OpenCodeAdapter) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := a.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", method, path, err)
	}

	return resp, nil
}

// decodeResponse reads and decodes a JSON response body.
func decodeResponse[T any](resp *http.Response) (T, error) {
	var result T
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return result, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// --- Adapter interface implementation ---

func (a *OpenCodeAdapter) GetStatus(ctx context.Context) (*Status, error) {
	resp, err := a.doRequest(ctx, "GET", "/global/health", nil)
	if err != nil {
		return &Status{
			Connected: false,
			Provider:  "opencode",
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return &Status{
			Connected: false,
			Provider:  "opencode",
		}, nil
	}

	var health struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return &Status{
			Connected: false,
			Provider:  "opencode",
		}, nil
	}

	return &Status{
		Connected: health.Healthy,
		Provider:  "opencode",
		Version:   health.Version,
	}, nil
}

// openCodeSession is the JSON shape returned by OpenCode's /session endpoints.
type openCodeSession struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectID"`
	Title     string `json:"title"`
	Version   string `json:"version"`
	Time      struct {
		Created int64 `json:"created"`
		Updated int64 `json:"updated"`
	} `json:"time"`
}

func (s *openCodeSession) toSession() Session {
	return Session{
		ID:        s.ID,
		Title:     s.Title,
		ProjectID: s.ProjectID,
		Time: &TimeInfo{
			Created: s.Time.Created,
			Updated: s.Time.Updated,
		},
	}
}

func (a *OpenCodeAdapter) ListSessions(ctx context.Context) ([]Session, error) {
	resp, err := a.doRequest(ctx, "GET", "/session", nil)
	if err != nil {
		return nil, err
	}

	ocSessions, err := decodeResponse[[]openCodeSession](resp)
	if err != nil {
		return nil, err
	}

	sessions := make([]Session, len(ocSessions))
	for i, ocs := range ocSessions {
		sessions[i] = ocs.toSession()
	}
	return sessions, nil
}

func (a *OpenCodeAdapter) CreateSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	body := map[string]interface{}{}
	if opts.Title != "" {
		body["title"] = opts.Title
	}
	if opts.ParentID != "" {
		body["parentID"] = opts.ParentID
	}

	resp, err := a.doRequest(ctx, "POST", "/session", body)
	if err != nil {
		return nil, err
	}

	ocs, err := decodeResponse[openCodeSession](resp)
	if err != nil {
		return nil, err
	}

	session := ocs.toSession()
	session.Mode = opts.Mode
	return &session, nil
}

func (a *OpenCodeAdapter) ResumeSession(ctx context.Context, id string) (*Session, error) {
	resp, err := a.doRequest(ctx, "GET", "/session/"+id, nil)
	if err != nil {
		return nil, err
	}

	ocs, err := decodeResponse[openCodeSession](resp)
	if err != nil {
		return nil, err
	}

	session := ocs.toSession()
	return &session, nil
}

func (a *OpenCodeAdapter) DeleteSession(ctx context.Context, id string) error {
	resp, err := a.doRequest(ctx, "DELETE", "/session/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (a *OpenCodeAdapter) AbortSession(ctx context.Context, sessionID string) error {
	resp, err := a.doRequest(ctx, "POST", "/session/"+sessionID+"/abort", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// SendMessage sends a message and waits for the full response (synchronous).
// Returns a channel of voilot Events parsed from the SSE event stream.
func (a *OpenCodeAdapter) SendMessage(ctx context.Context, sessionID string, message string) (<-chan Event, error) {
	events := make(chan Event, 64)

	// Subscribe to SSE events for this session before sending
	sseCh, err := a.SubscribeEvents(ctx)
	if err != nil {
		close(events)
		return events, err
	}

	// Send message asynchronously
	if err := a.SendMessageAsync(ctx, sessionID, message); err != nil {
		close(events)
		return events, err
	}

	// Filter SSE events for this session and forward to output channel
	go func() {
		defer close(events)
		for evt := range sseCh {
			if evt.SessionID != "" && evt.SessionID != sessionID {
				continue
			}
			events <- evt
			if evt.Type == EventDone {
				return
			}
		}
	}()

	return events, nil
}

// SendMessageAsync sends a message without waiting for a response.
// Use SubscribeEvents() to receive streaming events.
func (a *OpenCodeAdapter) SendMessageAsync(ctx context.Context, sessionID string, message string) error {
	body := map[string]interface{}{
		"parts": []map[string]string{
			{
				"type": "text",
				"text": message,
			},
		},
	}

	resp, err := a.doRequest(ctx, "POST", "/session/"+sessionID+"/prompt_async", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// prompt_async returns 204 No Content on success
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SubscribeEvents connects to the OpenCode SSE event stream and returns a channel
// of voilot Events. The channel is closed when the context is cancelled.
func (a *OpenCodeAdapter) SubscribeEvents(ctx context.Context) (<-chan Event, error) {
	ch := make(chan Event, 128)

	a.mu.Lock()
	a.subscribers[ch] = struct{}{}

	// Start the SSE reader if not already running
	if !a.sseRunning {
		a.sseRunning = true
		go a.runSSEReader()
	}
	a.mu.Unlock()

	// Remove subscriber when context is done
	go func() {
		<-ctx.Done()
		a.mu.Lock()
		delete(a.subscribers, ch)
		a.mu.Unlock()
		// Drain and close
		close(ch)
	}()

	return ch, nil
}

// broadcast sends an event to all current subscribers.
func (a *OpenCodeAdapter) broadcast(evt Event) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for ch := range a.subscribers {
		select {
		case ch <- evt:
		default:
			// Drop if subscriber is too slow
			log.Printf("Warning: dropping event for slow subscriber")
		}
	}
}

// runSSEReader maintains a persistent SSE connection to OpenCode's /event endpoint.
// It reconnects on failure and parses events into voilot Events.
func (a *OpenCodeAdapter) runSSEReader() {
	for {
		err := a.readSSEStream()
		if err != nil {
			log.Printf("SSE connection lost: %v — reconnecting in 2s", err)
		}

		a.mu.RLock()
		subscriberCount := len(a.subscribers)
		a.mu.RUnlock()

		if subscriberCount == 0 {
			a.mu.Lock()
			a.sseRunning = false
			a.mu.Unlock()
			return
		}

		time.Sleep(2 * time.Second)
	}
}

// readSSEStream opens a single SSE connection and reads events until it closes.
func (a *OpenCodeAdapter) readSSEStream() error {
	req, err := http.NewRequest("GET", a.baseURL+"/event", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Use a client without the default timeout for SSE (long-lived connection)
	sseClient := &http.Client{Timeout: 0}
	resp, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SSE HTTP %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Allow larger lines for SSE events with big payloads
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			continue
		}

		// Empty line signals end of event
		if line == "" && len(dataLines) > 0 {
			data := strings.Join(dataLines, "\n")
			dataLines = nil

			events := a.parseSSEData(data)
			for _, evt := range events {
				a.broadcast(evt)
			}
		}
	}

	return scanner.Err()
}

// parseSSEData converts a raw SSE data payload into voilot Event(s).
func (a *OpenCodeAdapter) parseSSEData(data string) []Event {
	var raw OpenCodeSSEEvent
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil
	}

	switch raw.Type {
	case "message.part.updated":
		return a.parsePartUpdate(raw.Properties)

	case "session.status":
		return a.parseSessionStatus(raw.Properties)

	case "session.created":
		return a.parseSessionCreated(raw.Properties)

	case "session.updated":
		return a.parseSessionUpdated(raw.Properties)

	case "session.error":
		return a.parseSessionError(raw.Properties)

	case "session.idle":
		var props struct {
			SessionID string `json:"sessionID"`
		}
		if json.Unmarshal(raw.Properties, &props) == nil {
			return []Event{{
				Type:      EventDone,
				SessionID: props.SessionID,
				Content:   "done",
			}}
		}

	case "message.updated":
		// Message-level updates (completed, error, etc.)
		return a.parseMessageUpdated(raw.Properties)

	case "server.connected":
		log.Println("SSE: connected to OpenCode server")
	}

	return nil
}

func (a *OpenCodeAdapter) parsePartUpdate(props json.RawMessage) []Event {
	var update OpenCodePartUpdate
	if err := json.Unmarshal(props, &update); err != nil {
		return nil
	}

	var part OpenCodePart
	if err := json.Unmarshal(update.Part, &part); err != nil {
		return nil
	}

	switch part.Type {
	case "text":
		evt := Event{
			Type:      EventText,
			SessionID: part.SessionID,
			MessageID: part.MessageID,
			PartID:    part.ID,
			Content:   part.Text,
			Delta:     update.Delta,
		}
		return []Event{evt}

	case "reasoning":
		return []Event{{
			Type:      EventThinking,
			SessionID: part.SessionID,
			MessageID: part.MessageID,
			PartID:    part.ID,
			Content:   part.Text,
			Delta:     update.Delta,
		}}

	case "tool":
		return a.parseToolPart(part, update.Delta)

	case "step-start":
		// Ignore step boundaries
		return nil

	case "step-finish":
		// Could emit a summary event here
		return nil

	default:
		return nil
	}
}

func (a *OpenCodeAdapter) parseToolPart(part OpenCodePart, delta string) []Event {
	if part.State == nil {
		return nil
	}

	var state OpenCodeToolState
	if err := json.Unmarshal(part.State, &state); err != nil {
		return nil
	}

	evt := Event{
		SessionID: part.SessionID,
		MessageID: part.MessageID,
		PartID:    part.ID,
		Meta: map[string]interface{}{
			"tool":   part.Tool,
			"status": state.Status,
		},
	}

	switch state.Status {
	case "running":
		evt.Type = EventToolUse
		evt.Content = fmt.Sprintf("Using tool: %s", part.Tool)
		if state.Title != "" {
			evt.Content = state.Title
		}
	case "completed":
		evt.Type = EventToolResult
		evt.Content = state.Output
		if state.Title != "" {
			evt.Meta["title"] = state.Title
		}
	case "error":
		evt.Type = EventError
		evt.Content = state.Error
	default:
		// pending — skip
		return nil
	}

	return []Event{evt}
}

func (a *OpenCodeAdapter) parseSessionStatus(props json.RawMessage) []Event {
	var status OpenCodeSessionStatus
	if err := json.Unmarshal(props, &status); err != nil {
		return nil
	}

	evt := Event{
		Type:      EventStatus,
		SessionID: status.SessionID,
		Content:   status.Status.Type,
		Meta: map[string]interface{}{
			"statusType": status.Status.Type,
		},
	}

	// When session goes idle after being busy, signal done
	if status.Status.Type == "idle" {
		return []Event{
			evt,
			{
				Type:      EventDone,
				SessionID: status.SessionID,
				Content:   "done",
			},
		}
	}

	return []Event{evt}
}

func (a *OpenCodeAdapter) parseSessionCreated(props json.RawMessage) []Event {
	var info OpenCodeSessionInfo
	if err := json.Unmarshal(props, &info); err != nil {
		return nil
	}

	var session openCodeSession
	if err := json.Unmarshal(info.Info, &session); err != nil {
		return nil
	}

	return []Event{{
		Type:      EventSessionCreated,
		SessionID: session.ID,
		Content:   session.Title,
	}}
}

func (a *OpenCodeAdapter) parseSessionUpdated(props json.RawMessage) []Event {
	var info OpenCodeSessionInfo
	if err := json.Unmarshal(props, &info); err != nil {
		return nil
	}

	var session openCodeSession
	if err := json.Unmarshal(info.Info, &session); err != nil {
		return nil
	}

	return []Event{{
		Type:      EventSessionUpdated,
		SessionID: session.ID,
		Content:   session.Title,
	}}
}

func (a *OpenCodeAdapter) parseMessageUpdated(props json.RawMessage) []Event {
	var info OpenCodeMessageInfo
	if err := json.Unmarshal(props, &info); err != nil {
		return nil
	}

	// Check if the message has an error
	var msg struct {
		ID        string `json:"id"`
		SessionID string `json:"sessionID"`
		Role      string `json:"role"`
		Error     *struct {
			Name string          `json:"name"`
			Data json.RawMessage `json:"data"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(info.Info, &msg); err != nil {
		return nil
	}

	if msg.Error != nil {
		return []Event{{
			Type:      EventError,
			SessionID: msg.SessionID,
			MessageID: msg.ID,
			Content:   fmt.Sprintf("Error (%s)", msg.Error.Name),
			Meta: map[string]interface{}{
				"errorName": msg.Error.Name,
			},
		}}
	}

	return nil
}

func (a *OpenCodeAdapter) parseSessionError(props json.RawMessage) []Event {
	var errInfo OpenCodeSessionError
	if err := json.Unmarshal(props, &errInfo); err != nil {
		return nil
	}

	content := "Session error"
	if errInfo.Error != nil {
		content = string(errInfo.Error)
	}

	return []Event{{
		Type:      EventError,
		SessionID: errInfo.SessionID,
		Content:   content,
	}}
}

// Verify interface compliance at compile time.
var _ Adapter = (*OpenCodeAdapter)(nil)
