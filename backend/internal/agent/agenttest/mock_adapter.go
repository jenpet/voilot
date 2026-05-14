package agenttest

import (
	"context"
	"fmt"
	"sync"

	"github.com/jenpet/voilot/internal/agent"
)

// MockAdapter is an in-memory fake implementing agent.Adapter for tests.
// It tracks method calls and returns configurable responses.
type MockAdapter struct {
	mu sync.Mutex

	// Configurable return values
	Sessions      []agent.Session
	Agents        []agent.AgentInfo
	Models        *agent.ModelCatalog
	Messages      []agent.HistoryMessage
	Status        *agent.Status
	CreateErr     error
	ListErr       error
	SendErr       error
	AbortErr      error
	DeleteErr     error
	ResumeErr     error
	PermissionErr error
	QuestionErr   error

	// SSE event channel: tests push events here, SubscribeEvents returns it.
	EventCh chan agent.Event

	// Call tracking
	AbortCalled      []string // session IDs
	DeleteCalled     []string
	SendCalled       []sendCall
	PermissionCalled []permissionCall
	QuestionCalled   []questionCall
	RejectCalled     []string
	CreateCalled     []agent.SessionOptions
	InitializeCalled []initializeCall
	SetAgentCalled   []setAgentCall
	SetModelCalled   []setModelCall

	// Per-session state
	sessionAgents map[string]string
	sessionModels map[string]string
	sessionBusy   map[string]bool
}

type sendCall struct {
	SessionID string
	Message   string
	Agent     string
	Model     string
}

type permissionCall struct {
	SessionID    string
	PermissionID string
	Response     string
	Remember     bool
}

type questionCall struct {
	RequestID string
	Answers   [][]string
}

type setAgentCall struct {
	SessionID string
	Agent     string
}

type setModelCall struct {
	SessionID string
	Model     string
}

type initializeCall struct {
	SessionID string
	Prompt    string
}

// Verify interface compliance.
var _ agent.Adapter = (*MockAdapter)(nil)

// NewMockAdapter creates a new MockAdapter with sensible defaults.
func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		EventCh:       make(chan agent.Event, 64),
		sessionAgents: make(map[string]string),
		sessionModels: make(map[string]string),
		sessionBusy:   make(map[string]bool),
		Status:        &agent.Status{Connected: true, Provider: "mock"},
	}
}

func (a *MockAdapter) CreateSession(_ context.Context, opts agent.SessionOptions) (*agent.Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.CreateCalled = append(a.CreateCalled, opts)
	if a.CreateErr != nil {
		return nil, a.CreateErr
	}
	s := &agent.Session{
		ID:    fmt.Sprintf("ses-%d", len(a.CreateCalled)),
		Title: opts.Title,
	}
	return s, nil
}

func (a *MockAdapter) InitializeSession(_ context.Context, sessionID string, prompt string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.InitializeCalled = append(a.InitializeCalled, initializeCall{sessionID, prompt})
	return nil
}

func (a *MockAdapter) ResumeSession(_ context.Context, id string) (*agent.Session, error) {
	if a.ResumeErr != nil {
		return nil, a.ResumeErr
	}
	for _, s := range a.Sessions {
		if s.ID == id {
			return &s, nil
		}
	}
	return &agent.Session{ID: id}, nil
}

func (a *MockAdapter) ListSessions(_ context.Context) ([]agent.Session, error) {
	if a.ListErr != nil {
		return nil, a.ListErr
	}
	return a.Sessions, nil
}

func (a *MockAdapter) DeleteSession(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.DeleteCalled = append(a.DeleteCalled, id)
	return a.DeleteErr
}

func (a *MockAdapter) GetMessages(_ context.Context, _ string) ([]agent.HistoryMessage, error) {
	return a.Messages, nil
}

func (a *MockAdapter) SendMessage(_ context.Context, sessionID string, message string) (<-chan agent.Event, error) {
	a.mu.Lock()
	a.SendCalled = append(a.SendCalled, sendCall{SessionID: sessionID, Message: message})
	a.mu.Unlock()
	if a.SendErr != nil {
		return nil, a.SendErr
	}
	ch := make(chan agent.Event)
	close(ch)
	return ch, nil
}

func (a *MockAdapter) SendMessageAsync(_ context.Context, sessionID, message, agentName, modelID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.SendCalled = append(a.SendCalled, sendCall{SessionID: sessionID, Message: message, Agent: agentName, Model: modelID})
	return a.SendErr
}

func (a *MockAdapter) AbortSession(_ context.Context, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.AbortCalled = append(a.AbortCalled, sessionID)
	return a.AbortErr
}

func (a *MockAdapter) ListAgents(_ context.Context) ([]agent.AgentInfo, error) {
	return a.Agents, nil
}

func (a *MockAdapter) SetSessionAgent(sessionID, agentName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.SetAgentCalled = append(a.SetAgentCalled, setAgentCall{sessionID, agentName})
	a.sessionAgents[sessionID] = agentName
}

func (a *MockAdapter) GetSessionAgent(sessionID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if v, ok := a.sessionAgents[sessionID]; ok {
		return v
	}
	return "planitect"
}

func (a *MockAdapter) ListModels(_ context.Context) (*agent.ModelCatalog, error) {
	return a.Models, nil
}

func (a *MockAdapter) SetSessionModel(sessionID, modelID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.SetModelCalled = append(a.SetModelCalled, setModelCall{sessionID, modelID})
	a.sessionModels[sessionID] = modelID
}

func (a *MockAdapter) GetSessionModel(sessionID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionModels[sessionID]
}

func (a *MockAdapter) GetSessionBusy(sessionID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionBusy[sessionID]
}

func (a *MockAdapter) GetStatus(_ context.Context) (*agent.Status, error) {
	return a.Status, nil
}

func (a *MockAdapter) SubscribeEvents(_ context.Context) (<-chan agent.Event, error) {
	return a.EventCh, nil
}

func (a *MockAdapter) RespondToPermission(_ context.Context, sessionID, permissionID, response string, remember bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.PermissionCalled = append(a.PermissionCalled, permissionCall{sessionID, permissionID, response, remember})
	return a.PermissionErr
}

func (a *MockAdapter) RespondToQuestion(_ context.Context, requestID string, answers [][]string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.QuestionCalled = append(a.QuestionCalled, questionCall{requestID, answers})
	return a.QuestionErr
}

func (a *MockAdapter) RejectQuestion(_ context.Context, requestID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.RejectCalled = append(a.RejectCalled, requestID)
	return a.QuestionErr
}
