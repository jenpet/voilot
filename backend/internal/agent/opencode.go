package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
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

	// Session model storage: model override per session (provider/model).
	modelMu       sync.RWMutex
	sessionModels map[string]string

	// Session last-used model storage: latest observed model per session.
	lastModelMu       sync.RWMutex
	sessionLastModels map[string]string

	// Track user message IDs to filter out echoed user messages from SSE.
	// Entries have a TTL and are periodically cleaned up to prevent unbounded growth.
	userMsgMu  sync.RWMutex
	userMsgIDs map[string]time.Time // messageID -> insertion time

	// Track part IDs that have received delta events, so we can skip
	// the redundant final full-text snapshot from message.part.updated.
	deltaPartMu  sync.RWMutex
	deltaPartIDs map[string]struct{} // partID -> exists

	// Track reasoning/thinking part IDs so their deltas can be suppressed.
	// Reasoning content is internal model chain-of-thought that should never
	// reach the frontend (not spoken via TTS, not rendered in the UI).
	reasoningPartMu  sync.RWMutex
	reasoningPartIDs map[string]struct{} // partID -> exists

	// Track per-session busy/idle status from session.status SSE events.
	// Used to answer "is this session busy?" on page reload / reconnect.
	sessionStatusMu sync.RWMutex
	sessionStatuses map[string]string // sessionID -> "idle" | "busy" | "retry"
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
		subscribers:       make(map[chan Event]struct{}),
		sessionModes:      make(map[string]SessionMode),
		sessionAgents:     make(map[string]string),
		sessionModels:     make(map[string]string),
		sessionLastModels: make(map[string]string),
		userMsgIDs:        make(map[string]time.Time),
		deltaPartIDs:      make(map[string]struct{}),
		reasoningPartIDs:  make(map[string]struct{}),
		sessionStatuses:   make(map[string]string),
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
	ParentID  string `json:"parentID"`
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
		ParentID:  s.ParentID,
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
		sessions[i].Model = a.GetSessionModel(sessions[i].ID)
		sessions[i].LastUsedModel = a.GetSessionLastUsedModel(sessions[i].ID)
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
	if opts.Model != "" {
		body["model"] = opts.Model
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
	session.Model = opts.Model
	session.LastUsedModel = a.GetSessionLastUsedModel(session.ID)
	// Store the mode and agent in our local maps
	a.SetSessionMode(session.ID, opts.Mode)
	if opts.Agent != "" {
		a.SetSessionAgent(session.ID, opts.Agent)
	}
	if opts.Model != "" {
		a.SetSessionModel(session.ID, opts.Model)
	}
	return &session, nil
}

// InitializeSession sends the provided prompt to the session as the first
// interaction. It looks up the session's agent and model internally.
func (a *OpenCodeAdapter) InitializeSession(ctx context.Context, sessionID string, prompt string) error {
	agentName := a.GetSessionAgent(sessionID)
	modelID := a.GetSessionModel(sessionID)
	return a.SendMessageAsync(ctx, sessionID, prompt, agentName, modelID)
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
	session.Model = a.GetSessionModel(id)
	lastUsed := a.GetSessionLastUsedModel(id)
	if lastUsed == "" {
		if messages, msgErr := a.GetMessages(ctx, id); msgErr == nil {
			for i := len(messages) - 1; i >= 0; i-- {
				meta := messages[i].Meta
				if meta == nil {
					continue
				}
				if model, ok := meta["model"].(string); ok && model != "" {
					lastUsed = model
					break
				}
			}
		}
		if lastUsed != "" {
			a.SetSessionLastUsedModel(id, lastUsed)
		}
	}
	session.LastUsedModel = lastUsed
	return &session, nil
}

// openCodeMessageResponse is the JSON shape of a message from OpenCode's /session/:id/message endpoint.
type openCodeMessageResponse struct {
	Info struct {
		ID         string `json:"id"`
		Role       string `json:"role"`
		ProviderID string `json:"providerID,omitempty"`
		ModelID    string `json:"modelID,omitempty"`
		Model      *struct {
			ProviderID string `json:"providerID"`
			ModelID    string `json:"modelID"`
		} `json:"model,omitempty"`
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
	firstUserSeen := false
	firstUserHidden := false
	secondUserSeen := false
	for _, ocMsg := range ocMessages {
		role := ocMsg.Info.Role
		timestamp := ocMsg.Info.Time.Created
		model := ""
		if ocMsg.Info.ProviderID != "" && ocMsg.Info.ModelID != "" {
			model = ocMsg.Info.ProviderID + "/" + ocMsg.Info.ModelID
		} else if ocMsg.Info.Model != nil && ocMsg.Info.Model.ProviderID != "" && ocMsg.Info.Model.ModelID != "" {
			model = ocMsg.Info.Model.ProviderID + "/" + ocMsg.Info.Model.ModelID
		}

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
				content := strings.Join(textParts, "\n")
				meta := map[string]interface{}{}
				if model != "" {
					meta["model"] = model
				}

				// Strip [Working in: ...] prefix from user messages
				if idx := strings.Index(content, "]\n\n"); idx != -1 && strings.HasPrefix(content, "[Working in: ") {
					content = content[idx+3:]
				}

				// Mark the auto-generated scoping message as hidden
				if !firstUserSeen {
					firstUserSeen = true
					if strings.HasPrefix(content, "You are working in ") && strings.Contains(content, "Briefly welcome the user") {
						meta["hidden"] = true
						firstUserHidden = true
					}
				} else {
					secondUserSeen = true
				}

				messages = append(messages, HistoryMessage{
					ID:        ocMsg.Info.ID,
					Role:      "user",
					Content:   content,
					Timestamp: timestamp,
					Meta:      meta,
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
					// Hide tool messages from the initial scoping response
					if firstUserHidden && !secondUserSeen {
						meta["hidden"] = true
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
				meta := map[string]interface{}{}
				if model != "" {
					meta["model"] = model
				}
				messages = append(messages, HistoryMessage{
					ID:        ocMsg.Info.ID,
					Role:      "assistant",
					Content:   strings.Join(textParts, "\n"),
					Timestamp: timestamp,
					Meta:      meta,
				})
			}
		}
	}

	for i := len(messages) - 1; i >= 0; i-- {
		meta := messages[i].Meta
		if meta == nil {
			continue
		}
		if model, ok := meta["model"].(string); ok && model != "" {
			a.SetSessionLastUsedModel(sessionID, model)
			break
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
	a.modelMu.Lock()
	delete(a.sessionModels, id)
	a.modelMu.Unlock()
	a.lastModelMu.Lock()
	delete(a.sessionLastModels, id)
	a.lastModelMu.Unlock()

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
	modelID := a.GetSessionModel(sessionID)
	if err := a.SendMessageAsync(ctx, sessionID, message, agentName, modelID); err != nil {
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

// GetSessionBusy returns true if the session is currently busy (agent processing).
func (a *OpenCodeAdapter) GetSessionBusy(sessionID string) bool {
	a.sessionStatusMu.RLock()
	defer a.sessionStatusMu.RUnlock()
	status, ok := a.sessionStatuses[sessionID]
	if !ok {
		return false // unknown session defaults to idle
	}
	return status == "busy" || status == "retry"
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

// SetSessionModel stores the active model override for a session.
func (a *OpenCodeAdapter) SetSessionModel(sessionID string, modelID string) {
	a.modelMu.Lock()
	defer a.modelMu.Unlock()
	if modelID == "" {
		delete(a.sessionModels, sessionID)
		return
	}
	a.sessionModels[sessionID] = modelID
}

// GetSessionModel returns the active model override for a session.
// Empty string means "use OpenCode default model".
func (a *OpenCodeAdapter) GetSessionModel(sessionID string) string {
	a.modelMu.RLock()
	defer a.modelMu.RUnlock()
	if model, ok := a.sessionModels[sessionID]; ok {
		return model
	}
	return ""
}

// SetSessionLastUsedModel stores the most recently observed model for a session.
func (a *OpenCodeAdapter) SetSessionLastUsedModel(sessionID string, modelID string) {
	a.lastModelMu.Lock()
	defer a.lastModelMu.Unlock()
	if modelID == "" {
		delete(a.sessionLastModels, sessionID)
		return
	}
	a.sessionLastModels[sessionID] = modelID
}

// GetSessionLastUsedModel returns the most recently observed model for a session.
func (a *OpenCodeAdapter) GetSessionLastUsedModel(sessionID string) string {
	a.lastModelMu.RLock()
	defer a.lastModelMu.RUnlock()
	if model, ok := a.sessionLastModels[sessionID]; ok {
		return model
	}
	return ""
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

type openCodeProvider struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Models json.RawMessage `json:"models"`
}

type openCodeModelDef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type openCodeProviderResponse struct {
	All       []openCodeProvider `json:"all"`
	Default   map[string]string  `json:"default"`
	Provider  []openCodeProvider `json:"providers"`
	Connected []string           `json:"connected"`
}

func parseOpenCodeModelEntries(raw json.RawMessage) map[string]openCodeModelDef {
	if len(raw) == 0 {
		return map[string]openCodeModelDef{}
	}

	var asMap map[string]openCodeModelDef
	if err := json.Unmarshal(raw, &asMap); err == nil {
		return asMap
	}

	var asArray []openCodeModelDef
	if err := json.Unmarshal(raw, &asArray); err == nil {
		out := make(map[string]openCodeModelDef, len(asArray))
		for _, item := range asArray {
			id := item.ID
			if id == "" {
				continue
			}
			out[id] = item
		}
		return out
	}

	return map[string]openCodeModelDef{}
}

func (a *OpenCodeAdapter) fetchProviderCatalog(ctx context.Context) (*openCodeProviderResponse, error) {
	resp, err := a.doRequest(ctx, "GET", "/provider", nil)
	if err == nil {
		catalog, decodeErr := decodeResponse[openCodeProviderResponse](resp)
		if decodeErr == nil {
			return &catalog, nil
		}
	}

	resp, err = a.doRequest(ctx, "GET", "/config/providers", nil)
	if err != nil {
		return nil, err
	}
	catalog, err := decodeResponse[openCodeProviderResponse](resp)
	if err != nil {
		return nil, err
	}
	return &catalog, nil
}

// ListModels fetches available models and the default model from OpenCode.
func (a *OpenCodeAdapter) ListModels(ctx context.Context) (*ModelCatalog, error) {
	catalogRaw, err := a.fetchProviderCatalog(ctx)
	if err != nil {
		return nil, err
	}

	providers := catalogRaw.All
	if len(providers) == 0 {
		providers = catalogRaw.Provider
	}

	connected := make(map[string]struct{}, len(catalogRaw.Connected))
	for _, providerID := range catalogRaw.Connected {
		if providerID == "" {
			continue
		}
		connected[providerID] = struct{}{}
	}

	seen := make(map[string]ModelInfo)
	for _, provider := range providers {
		providerID := provider.ID
		if providerID == "" {
			continue
		}
		if len(connected) > 0 {
			if _, ok := connected[providerID]; !ok {
				continue
			}
		}
		providerName := provider.Name
		if providerName == "" {
			providerName = providerID
		}

		models := parseOpenCodeModelEntries(provider.Models)
		for modelKey, modelDef := range models {
			modelID := modelKey
			if modelID == "" {
				modelID = modelDef.ID
			}
			if modelID == "" {
				continue
			}
			fullID := providerID + "/" + modelID
			name := modelDef.Name
			if name == "" {
				name = modelID
			}
			seen[fullID] = ModelInfo{
				ID:           fullID,
				Name:         name,
				ProviderID:   providerID,
				ProviderName: providerName,
			}
		}
	}

	models := make([]ModelInfo, 0, len(seen))
	for _, model := range seen {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].ProviderName == models[j].ProviderName {
			return models[i].Name < models[j].Name
		}
		return models[i].ProviderName < models[j].ProviderName
	})

	defaultModel := ""
	resp, err := a.doRequest(ctx, "GET", "/config", nil)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var cfg struct {
				Model string `json:"model"`
			}
			if decodeErr := json.NewDecoder(resp.Body).Decode(&cfg); decodeErr == nil {
				defaultModel = cfg.Model
			}
		}
	}

	// If /config does not expose a global default model, keep this empty.
	// The /provider default map is provider-specific and does not represent a
	// single global default model, and iterating that map is nondeterministic.

	return &ModelCatalog{
		Models:       models,
		DefaultModel: defaultModel,
	}, nil
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
func (a *OpenCodeAdapter) SendMessageAsync(ctx context.Context, sessionID string, message string, agentName string, modelID string) error {
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
	if modelID != "" {
		providerID := ""
		providerModelID := modelID
		if idx := strings.Index(modelID, "/"); idx > 0 && idx < len(modelID)-1 {
			providerID = modelID[:idx]
			providerModelID = modelID[idx+1:]
		}
		if providerID != "" {
			body["model"] = map[string]string{
				"providerID": providerID,
				"modelID":    providerModelID,
			}
		}
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
			slog.Warn("dropping event for slow subscriber")
		}
	}
}

// runSSEReader maintains a persistent SSE connection to OpenCode's /event endpoint.
// It reconnects on failure and parses events into voilot Events.
func (a *OpenCodeAdapter) runSSEReader() {
	for {
		err := a.readSSEStream()
		if err != nil {
			slog.Warn("SSE connection lost, reconnecting", "error", err, "delay", "2s")
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
		slog.Info("SSE connected to OpenCode server")

	case "permission.asked":
		return a.parsePermissionAsked(raw.Properties)

	case "permission.replied":
		return a.parsePermissionReplied(raw.Properties)

	case "question.asked":
		return a.parseQuestionAsked(raw.Properties)

	case "question.replied":
		return a.parseQuestionReplied(raw.Properties)

	case "question.rejected":
		return a.parseQuestionRejected(raw.Properties)

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
		// Track reasoning part IDs so their streaming deltas are also suppressed.
		a.reasoningPartMu.Lock()
		a.reasoningPartIDs[part.ID] = struct{}{}
		a.reasoningPartMu.Unlock()
		return nil

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
			slog.Info("SSE forwarding unknown part type", "type", part.Type, "id", part.ID, "textLen", len(part.Text))
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
		slog.Debug("SSE ignoring unknown part type with no text", "type", part.Type, "id", part.ID)
		return nil
	}
}

func (a *OpenCodeAdapter) parseToolPart(part OpenCodePart, delta string) []Event {
	// The "question" tool is handled via dedicated question.asked / question.replied
	// SSE events, similar to how permissions work. Suppress the generic tool_use /
	// tool_result events to avoid duplicating the question in the chat UI.
	if part.Tool == "question" {
		return nil
	}

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
		// Emit as tool_result (not error) so the UI updates the tool group
		// without triggering redundant system error messages and TTS
		// announcements. This covers permission denials and aborted tools.
		evt.Type = EventToolResult
		evt.Content = state.Error
		evt.Meta["error"] = state.Error
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

	// Skip deltas for reasoning/thinking parts
	a.reasoningPartMu.RLock()
	_, isReasoning := a.reasoningPartIDs[delta.PartID]
	a.reasoningPartMu.RUnlock()
	if isReasoning {
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

	// Store the latest status for this session so the frontend can
	// query it on reconnect / page reload.
	a.sessionStatusMu.Lock()
	a.sessionStatuses[status.SessionID] = status.Status.Type
	a.sessionStatusMu.Unlock()

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
		ID         string `json:"id"`
		SessionID  string `json:"sessionID"`
		Role       string `json:"role"`
		ProviderID string `json:"providerID,omitempty"`
		ModelID    string `json:"modelID,omitempty"`
		Model      *struct {
			ProviderID string `json:"providerID"`
			ModelID    string `json:"modelID"`
		} `json:"model,omitempty"`
		Error *struct {
			Name string          `json:"name"`
			Data json.RawMessage `json:"data"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(info.Info, &msg); err != nil {
		return nil
	}

	setLastUsedModel := func(model string) []Event {
		if model == "" {
			return nil
		}
		if a.GetSessionLastUsedModel(msg.SessionID) == model {
			return nil
		}
		a.SetSessionLastUsedModel(msg.SessionID, model)
		return []Event{{
			Type:      EventSessionUpdated,
			SessionID: msg.SessionID,
			Meta: map[string]interface{}{
				"lastUsedModel": model,
			},
		}}
	}

	assistantModel := ""
	if msg.ProviderID != "" && msg.ModelID != "" {
		assistantModel = msg.ProviderID + "/" + msg.ModelID
	}
	userModel := ""
	if msg.Model != nil && msg.Model.ProviderID != "" && msg.Model.ModelID != "" {
		userModel = msg.Model.ProviderID + "/" + msg.Model.ModelID
	}

	// Track user message IDs so we can skip their part updates
	if msg.Role == "user" {
		a.userMsgMu.Lock()
		a.userMsgIDs[msg.ID] = time.Now()
		a.userMsgMu.Unlock()
		return setLastUsedModel(userModel)
	}

	modelEvents := setLastUsedModel(assistantModel)

	if msg.Error != nil {
		// Suppress MessageAbortedError — this is a downstream consequence of
		// permission denials or user-initiated aborts and carries no useful
		// information beyond what the permission_replied or tool_result already
		// conveys. Other error types (rate limits, API errors) still propagate.
		if msg.Error.Name == "MessageAbortedError" {
			return modelEvents
		}
		errEvt := Event{
			Type:      EventError,
			SessionID: msg.SessionID,
			MessageID: msg.ID,
			Content:   fmt.Sprintf("Error (%s)", msg.Error.Name),
			Meta: map[string]interface{}{
				"errorName": msg.Error.Name,
			},
		}
		return append(modelEvents, errEvt)
	}

	return modelEvents
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
		slog.Error("failed to parse permission.asked", "error", err)
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
		slog.Error("failed to parse permission.replied", "error", err)
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

// ─── Question Handling ──────────────────────────────────────────

// parseQuestionAsked processes a "question.asked" SSE event from OpenCode.
// Multi-question batches are split into one EventQuestionRequest per question
// so the frontend renders each as an individual chat bubble.
func (a *OpenCodeAdapter) parseQuestionAsked(props json.RawMessage) []Event {
	var q OpenCodeQuestion
	if err := json.Unmarshal(props, &q); err != nil {
		slog.Error("SSE failed to parse question.asked", "error", err)
		return nil
	}

	if len(q.Questions) == 0 {
		return nil
	}

	total := len(q.Questions)
	events := make([]Event, 0, total)
	for i, item := range q.Questions {
		// Build a human-readable options list for TTS/display fallback
		optionLabels := make([]string, len(item.Options))
		for j, opt := range item.Options {
			optionLabels[j] = opt.Label
		}

		// Serialize the options array for the frontend
		optionsList := make([]interface{}, len(item.Options))
		for j, opt := range item.Options {
			optionsList[j] = map[string]interface{}{
				"label":       opt.Label,
				"description": opt.Description,
			}
		}

		events = append(events, Event{
			Type:      EventQuestionRequest,
			SessionID: q.SessionID,
			Content:   item.Question,
			Meta: map[string]interface{}{
				"questionId":     q.ID,
				"questionIndex":  i,
				"totalQuestions": total,
				"header":         item.Header,
				"options":        optionsList,
				"multiple":       item.Multiple,
			},
		})
	}

	return events
}

// parseQuestionReplied processes a "question.replied" SSE event.
func (a *OpenCodeAdapter) parseQuestionReplied(props json.RawMessage) []Event {
	var reply OpenCodeQuestionReply
	if err := json.Unmarshal(props, &reply); err != nil {
		slog.Error("SSE failed to parse question.replied", "error", err)
		return nil
	}

	// Flatten answers into a readable summary for the event content
	summary := ""
	for i, ans := range reply.Answers {
		if i > 0 {
			summary += "; "
		}
		summary += strings.Join(ans, ", ")
	}

	return []Event{{
		Type:      EventQuestionReplied,
		SessionID: reply.SessionID,
		Content:   summary,
		Meta: map[string]interface{}{
			"questionId": reply.RequestID,
			"answers":    reply.Answers,
			"rejected":   false,
		},
	}}
}

// parseQuestionRejected processes a "question.rejected" SSE event.
func (a *OpenCodeAdapter) parseQuestionRejected(props json.RawMessage) []Event {
	var rejected OpenCodeQuestionRejected
	if err := json.Unmarshal(props, &rejected); err != nil {
		slog.Error("SSE failed to parse question.rejected", "error", err)
		return nil
	}

	return []Event{{
		Type:      EventQuestionReplied,
		SessionID: rejected.SessionID,
		Content:   "Question dismissed",
		Meta: map[string]interface{}{
			"questionId": rejected.RequestID,
			"rejected":   true,
		},
	}}
}

// RespondToQuestion sends answers to a pending question prompt via the OpenCode API.
// The endpoint is NOT session-scoped: POST /question/{requestID}/reply.
func (a *OpenCodeAdapter) RespondToQuestion(ctx context.Context, requestID string, answers [][]string) error {
	body := map[string]interface{}{
		"answers": answers,
	}

	resp, err := a.doRequest(ctx, "POST", "/question/"+requestID+"/reply", body)
	if err != nil {
		return fmt.Errorf("respond to question: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// RejectQuestion dismisses a pending question prompt via the OpenCode API.
func (a *OpenCodeAdapter) RejectQuestion(ctx context.Context, requestID string) error {
	resp, err := a.doRequest(ctx, "POST", "/question/"+requestID+"/reject", nil)
	if err != nil {
		return fmt.Errorf("reject question: %w", err)
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
