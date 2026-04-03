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

	// Session mode storage (voilot-level, not forwarded to OpenCode).
	modeMu       sync.RWMutex
	sessionModes map[string]SessionMode

	// Session agent storage: which agent is active per session.
	agentMu       sync.RWMutex
	sessionAgents map[string]string

	// Track user message IDs to filter out echoed user messages from SSE.
	// Entries have a TTL and are periodically cleaned up to prevent unbounded growth.
	userMsgMu  sync.RWMutex
	userMsgIDs map[string]time.Time // messageID -> insertion time

	// Track part IDs that have received delta events, so we can skip
	// the redundant final full-text snapshot from message.part.updated.
	deltaPartMu  sync.RWMutex
	deltaPartIDs map[string]struct{} // partID -> exists
}

// userMsgIDTTL is how long user message IDs are retained for filtering.
const userMsgIDTTL = 30 * time.Minute

// userMsgIDCleanupInterval is how often the cleanup goroutine runs.
const userMsgIDCleanupInterval = 5 * time.Minute

// NewOpenCodeAdapter creates a new adapter pointing at the given OpenCode server URL.
func NewOpenCodeAdapter(baseURL string) *OpenCodeAdapter {
	a := &OpenCodeAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		subscribers:   make(map[chan Event]struct{}),
		sessionModes:  make(map[string]SessionMode),
		sessionAgents: make(map[string]string),
		userMsgIDs:    make(map[string]time.Time),
		deltaPartIDs:  make(map[string]struct{}),
	}

	// Start background cleanup of expired user message IDs.
	go a.cleanupUserMsgIDs()

	return a
}

// cleanupUserMsgIDs periodically removes user message IDs older than the TTL.
func (a *OpenCodeAdapter) cleanupUserMsgIDs() {
	ticker := time.NewTicker(userMsgIDCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-userMsgIDTTL)
		a.userMsgMu.Lock()
		for id, insertedAt := range a.userMsgIDs {
			if insertedAt.Before(cutoff) {
				delete(a.userMsgIDs, id)
			}
		}
		a.userMsgMu.Unlock()
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
		sessions[i].Mode = a.GetSessionMode(sessions[i].ID)
		sessions[i].Agent = a.GetSessionAgent(sessions[i].ID)
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
	session.Agent = opts.Agent
	// Store the mode and agent in our local maps
	a.SetSessionMode(session.ID, opts.Mode)
	if opts.Agent != "" {
		a.SetSessionAgent(session.ID, opts.Agent)
	}
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
	session.Mode = a.GetSessionMode(id)
	session.Agent = a.GetSessionAgent(id)
	return &session, nil
}

// openCodeMessageResponse is the JSON shape of a message from OpenCode's /session/:id/message endpoint.
type openCodeMessageResponse struct {
	Info struct {
		ID   string `json:"id"`
		Role string `json:"role"`
		Time struct {
			Created   int64 `json:"created"`
			Completed int64 `json:"completed,omitempty"`
		} `json:"time"`
	} `json:"info"`
	Parts []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text,omitempty"`
		MessageID string          `json:"messageID,omitempty"`
		Tool      string          `json:"tool,omitempty"`
		CallID    string          `json:"callID,omitempty"`
		State     json.RawMessage `json:"state,omitempty"`
	} `json:"parts"`
}

func (a *OpenCodeAdapter) GetMessages(ctx context.Context, sessionID string) ([]HistoryMessage, error) {
	resp, err := a.doRequest(ctx, "GET", "/session/"+sessionID+"/message", nil)
	if err != nil {
		return nil, err
	}

	ocMessages, err := decodeResponse[[]openCodeMessageResponse](resp)
	if err != nil {
		return nil, err
	}

	var messages []HistoryMessage
	for _, ocMsg := range ocMessages {
		role := ocMsg.Info.Role
		timestamp := ocMsg.Info.Time.Created

		switch role {
		case "user":
			// Extract text from parts
			var textParts []string
			for _, part := range ocMsg.Parts {
				if part.Type == "text" && part.Text != "" {
					textParts = append(textParts, part.Text)
				}
			}
			if len(textParts) > 0 {
				messages = append(messages, HistoryMessage{
					ID:        ocMsg.Info.ID,
					Role:      "user",
					Content:   strings.Join(textParts, "\n"),
					Timestamp: timestamp,
				})
			}
		case "assistant":
			// Extract text parts; skip step-start, step-finish, and other non-content parts
			var textParts []string
			for _, part := range ocMsg.Parts {
				switch part.Type {
				case "text":
					if part.Text != "" {
						textParts = append(textParts, part.Text)
					}
				case "tool":
					// Parse tool state for rich metadata
					meta := map[string]interface{}{}
					if part.Tool != "" {
						meta["tool"] = part.Tool
					}
					toolType := "tool_use"
					content := part.Text
					if part.State != nil {
						var state OpenCodeToolState
						if json.Unmarshal(part.State, &state) == nil {
							meta["status"] = state.Status
							if state.Title != "" {
								meta["title"] = state.Title
							}
							// Completed and error tools are results
							if state.Status == "completed" || state.Status == "error" {
								toolType = "tool_result"
								if state.Output != "" {
									content = truncateString(state.Output, 500)
								}
							}
						}
					}
					partID := part.MessageID + "-" + part.Type
					if part.CallID != "" {
						partID = part.MessageID + "-" + part.CallID
					}
					messages = append(messages, HistoryMessage{
						ID:        partID,
						Role:      "assistant",
						Content:   content,
						Timestamp: timestamp,
						Type:      toolType,
						Meta:      meta,
					})
				}
			}
			if len(textParts) > 0 {
				messages = append(messages, HistoryMessage{
					ID:        ocMsg.Info.ID,
					Role:      "assistant",
					Content:   strings.Join(textParts, "\n"),
					Timestamp: timestamp,
				})
			}
		}
	}

	return messages, nil
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

	// Clean up stored mode and agent
	a.modeMu.Lock()
	delete(a.sessionModes, id)
	a.modeMu.Unlock()
	a.agentMu.Lock()
	delete(a.sessionAgents, id)
	a.agentMu.Unlock()

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

	// Send message asynchronously — response events come via SSE
	agentName := a.GetSessionAgent(sessionID)
	if err := a.SendMessageAsync(ctx, sessionID, message, agentName); err != nil {
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

// SetSessionMode stores the mode for a session.
func (a *OpenCodeAdapter) SetSessionMode(sessionID string, mode SessionMode) {
	a.modeMu.Lock()
	defer a.modeMu.Unlock()
	a.sessionModes[sessionID] = mode
}

// GetSessionMode returns the mode for a session, defaulting to ModePlan.
func (a *OpenCodeAdapter) GetSessionMode(sessionID string) SessionMode {
	a.modeMu.RLock()
	defer a.modeMu.RUnlock()
	if mode, ok := a.sessionModes[sessionID]; ok {
		return mode
	}
	return ModePlan
}

// SetSessionAgent stores the active agent for a session.
func (a *OpenCodeAdapter) SetSessionAgent(sessionID string, agentName string) {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	a.sessionAgents[sessionID] = agentName
}

// GetSessionAgent returns the active agent for a session, defaulting to "planitect".
func (a *OpenCodeAdapter) GetSessionAgent(sessionID string) string {
	a.agentMu.RLock()
	defer a.agentMu.RUnlock()
	if agent, ok := a.sessionAgents[sessionID]; ok {
		return agent
	}
	return "planitect"
}

// openCodeAgent is the JSON shape returned by OpenCode's GET /agent endpoint.
type openCodeAgent struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color,omitempty"`
	Mode        string `json:"mode"`   // "primary" or "subagent"
	Native      bool   `json:"native"` // true for built-in agents
	Hidden      bool   `json:"hidden,omitempty"`
}

// ListAgents fetches available agents from OpenCode and filters to user-facing ones.
func (a *OpenCodeAdapter) ListAgents(ctx context.Context) ([]AgentInfo, error) {
	resp, err := a.doRequest(ctx, "GET", "/agent", nil)
	if err != nil {
		return nil, err
	}

	ocAgents, err := decodeResponse[[]openCodeAgent](resp)
	if err != nil {
		return nil, err
	}

	var agents []AgentInfo
	for _, oc := range ocAgents {
		// Skip hidden agents (compaction, title, summary)
		if oc.Hidden {
			continue
		}
		// Skip subagents (general, explore) — they are internal to the LLM
		if oc.Mode == "subagent" {
			continue
		}
		// Skip the built-in "plan" agent — voilot uses "planitect" instead
		if oc.Name == "plan" {
			continue
		}
		agents = append(agents, AgentInfo{
			Name:        oc.Name,
			Description: oc.Description,
			Color:       oc.Color,
		})
	}
	return agents, nil
}

// SendMessageAsync sends a message without waiting for a response.
// Use SubscribeEvents() to receive streaming events.
// If agentName is non-empty, that agent is selected in OpenCode.
func (a *OpenCodeAdapter) SendMessageAsync(ctx context.Context, sessionID string, message string, agentName string) error {
	body := map[string]interface{}{
		"parts": []map[string]string{
			{
				"type": "text",
				"text": message,
			},
		},
	}

	// Select the specified agent in OpenCode.
	if agentName != "" {
		body["agent"] = agentName
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

	case "message.part.delta":
		return a.parsePartDelta(raw.Properties)

	case "session.status":
		return a.parseSessionStatus(raw.Properties)

	case "session.created":
		return a.parseSessionCreated(raw.Properties)

	case "session.updated":
		return a.parseSessionUpdated(raw.Properties)

	case "session.error":
		return a.parseSessionError(raw.Properties)

	case "session.idle":
		// Duplicate of session.status(idle) which already emits EventDone — skip.
		return nil

	case "message.updated":
		// Track user vs assistant messages and handle errors
		return a.parseMessageUpdated(raw.Properties)

	case "server.connected":
		log.Println("SSE: connected to OpenCode server")

	case "permission.asked":
		return a.parsePermissionAsked(raw.Properties)

	case "permission.replied":
		return a.parsePermissionReplied(raw.Properties)

	case "server.heartbeat", "session.diff":
		// Ignore heartbeats and diffs
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

	// Skip updates for user messages (echoed back by OpenCode)
	a.userMsgMu.RLock()
	_, isUserMsg := a.userMsgIDs[part.MessageID]
	a.userMsgMu.RUnlock()
	if isUserMsg {
		return nil
	}

	switch part.Type {
	case "text":
		// Text content is streamed via message.part.delta events.
		// message.part.updated fires with initial empty text and final full text.
		if part.Text == "" {
			return nil // Skip initial empty text update
		}
		// If deltas were already streamed for this part, skip the final
		// full-text snapshot — the frontend has already accumulated the
		// same content from deltas and this would be redundant.
		a.deltaPartMu.RLock()
		_, hadDeltas := a.deltaPartIDs[part.ID]
		a.deltaPartMu.RUnlock()
		if hadDeltas {
			// Clean up: remove from tracking since the part is now complete.
			a.deltaPartMu.Lock()
			delete(a.deltaPartIDs, part.ID)
			a.deltaPartMu.Unlock()
			return nil
		}
		// No deltas were sent (rare edge case) — emit full content
		evt := Event{
			Type:      EventText,
			SessionID: part.SessionID,
			MessageID: part.MessageID,
			PartID:    part.ID,
			Content:   part.Text,
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
		// Unknown part type — log for diagnostics and forward if it carries text.
		// OpenCode defines additional part types (agent, subtask, file, snapshot,
		// patch, retry, compaction, etc.) that may carry user-facing content such
		// as agent option prompts. Rather than silently dropping them, surface any
		// text content as a regular EventText so the frontend can display it and
		// TTS can speak it.
		if part.Text != "" {
			log.Printf("SSE: forwarding unknown part type %q (id=%s) with %d chars of text", part.Type, part.ID, len(part.Text))
			// Apply the same delta-dedup logic as the "text" case: if deltas
			// already streamed this part's content, the final snapshot is redundant.
			a.deltaPartMu.RLock()
			_, hadDeltas := a.deltaPartIDs[part.ID]
			a.deltaPartMu.RUnlock()
			if hadDeltas {
				a.deltaPartMu.Lock()
				delete(a.deltaPartIDs, part.ID)
				a.deltaPartMu.Unlock()
				return nil
			}
			return []Event{{
				Type:      EventText,
				SessionID: part.SessionID,
				MessageID: part.MessageID,
				PartID:    part.ID,
				Content:   part.Text,
			}}
		}
		log.Printf("SSE: ignoring unknown part type %q (id=%s) with no text content", part.Type, part.ID)
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
		// Truncate tool output to avoid sending massive payloads over WebSocket.
		// The UI already truncates at display time, but this saves bandwidth.
		evt.Content = truncateString(state.Output, 500)
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

// parsePartDelta handles "message.part.delta" SSE events — streaming text deltas.
func (a *OpenCodeAdapter) parsePartDelta(props json.RawMessage) []Event {
	var delta OpenCodePartDelta
	if err := json.Unmarshal(props, &delta); err != nil {
		return nil
	}

	// Skip deltas for user messages
	a.userMsgMu.RLock()
	_, isUserMsg := a.userMsgIDs[delta.MessageID]
	a.userMsgMu.RUnlock()
	if isUserMsg {
		return nil
	}

	// Only handle text field deltas
	if delta.Field != "text" {
		return nil
	}

	// Track that this part has received deltas — the final message.part.updated
	// snapshot for this part is redundant and will be suppressed.
	a.deltaPartMu.Lock()
	a.deltaPartIDs[delta.PartID] = struct{}{}
	a.deltaPartMu.Unlock()

	return []Event{{
		Type:      EventText,
		SessionID: delta.SessionID,
		MessageID: delta.MessageID,
		PartID:    delta.PartID,
		Delta:     delta.Delta,
	}}
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

	// Parse message to check role and errors
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

	// Track user message IDs so we can skip their part updates
	if msg.Role == "user" {
		a.userMsgMu.Lock()
		a.userMsgIDs[msg.ID] = time.Now()
		a.userMsgMu.Unlock()
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

// parsePermissionUpdated handles "permission.updated" SSE events.
// These fire when a tool needs user approval (e.g. external_directory, bash).
func (a *OpenCodeAdapter) parsePermissionAsked(props json.RawMessage) []Event {
	var perm OpenCodePermission
	if err := json.Unmarshal(props, &perm); err != nil {
		log.Printf("Failed to parse permission.asked: %v", err)
		return nil
	}

	// Build a human-readable title from the permission type and metadata
	title := perm.Permission
	if filepath, ok := perm.Metadata["filepath"].(string); ok {
		title = perm.Permission + ": " + filepath
	}

	meta := map[string]interface{}{
		"permissionId":   perm.ID,
		"permissionType": perm.Permission,
		"title":          title,
	}
	if perm.Tool.CallID != "" {
		meta["callID"] = perm.Tool.CallID
	}
	if len(perm.Patterns) > 0 {
		meta["pattern"] = perm.Patterns
	}
	if perm.Metadata != nil {
		meta["metadata"] = perm.Metadata
	}

	return []Event{{
		Type:      EventPermissionRequest,
		SessionID: perm.SessionID,
		MessageID: perm.Tool.MessageID,
		Content:   title,
		Meta:      meta,
	}}
}

// parsePermissionReplied handles "permission.replied" SSE events.
// These fire when someone (this client, TUI, or another client) responds to a permission prompt.
func (a *OpenCodeAdapter) parsePermissionReplied(props json.RawMessage) []Event {
	var reply OpenCodePermissionReply
	if err := json.Unmarshal(props, &reply); err != nil {
		log.Printf("Failed to parse permission.replied: %v", err)
		return nil
	}

	return []Event{{
		Type:      EventPermissionReplied,
		SessionID: reply.SessionID,
		Content:   reply.Reply,
		Meta: map[string]interface{}{
			"permissionId": reply.RequestID,
			"response":     reply.Reply,
		},
	}}
}

// RespondToPermission sends a response to a pending permission prompt via the OpenCode API.
func (a *OpenCodeAdapter) RespondToPermission(ctx context.Context, sessionID, permissionID, response string, remember bool) error {
	body := map[string]interface{}{
		"response": response,
	}
	if remember {
		body["remember"] = true
	}

	resp, err := a.doRequest(ctx, "POST", "/session/"+sessionID+"/permissions/"+permissionID, body)
	if err != nil {
		return fmt.Errorf("respond to permission: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// truncateString truncates s to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// Verify interface compliance at compile time.
var _ Adapter = (*OpenCodeAdapter)(nil)
