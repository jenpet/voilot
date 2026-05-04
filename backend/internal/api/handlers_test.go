package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/agent/agenttest"
	"github.com/jenpet/voilot/internal/api"
	"github.com/jenpet/voilot/internal/sessionmap"
)

// testServer creates a Server backed by a MockProvider registry and a temp session map.
func testServer(t *testing.T, opts ...agent.RegistryOption) (*api.Server, *agenttest.MockProvider, *sessionmap.Map) {
	t.Helper()
	p := agenttest.NewMockProvider()
	pidDir := t.TempDir()
	providers := map[string]agent.Provider{"mock": p}
	reg, err := agent.NewProviderRegistry(providers, "mock", pidDir, opts...)
	if err != nil {
		t.Fatalf("NewProviderRegistry: %v", err)
	}
	t.Cleanup(func() { reg.Close() })

	mapFile := fmt.Sprintf("%s/session-map.json", t.TempDir())
	sesMap, err := sessionmap.New(mapFile)
	if err != nil {
		t.Fatalf("sessionmap.New: %v", err)
	}

	srv := api.NewServer(reg, nil, nil, nil, sesMap, nil)
	return srv, p, sesMap
}

// --- resolveAdapterForWorktree pattern ---

func TestHandleListSessions_RequiresWorktree(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleListSessions_WithWorktree(t *testing.T) {
	srv, p, sesMap := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Pre-configure mock to return sessions.
	p.NewAdapterFunc = func(baseURL string) agent.Adapter {
		a := agenttest.NewMockAdapter()
		a.Sessions = []agent.Session{
			{ID: "s1", Title: "test session"},
			{ID: "s2", Title: "other worktree session"},
		}
		return a
	}

	// Map s1 to the requested worktree, s2 to a different one.
	sesMap.SetEntry("s1", sessionmap.Entry{WorktreePath: "/worktree/a", Provider: "mock"})
	sesMap.SetEntry("s2", sessionmap.Entry{WorktreePath: "/worktree/b", Provider: "mock"})

	resp, err := http.Get(ts.URL + "/api/sessions?worktree=/worktree/a")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var sessions []json.RawMessage
	json.NewDecoder(resp.Body).Decode(&sessions)
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestHandleCreateSession_RequiresWorktreePath(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"title": "test"})
	resp, err := http.Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleCreateSession_Success(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{
		"worktreePath": "/worktree/a",
		"title":        "new session",
	})
	resp, err := http.Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

// --- resolveAdapter pattern (session ID lookup) ---

func TestHandleGetSession_UnknownSession(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/sessions/unknown-id")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandleGetSession_ValidSession(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create a session first (establishes the session map entry).
	body, _ := json.Marshal(map[string]string{
		"worktreePath": "/worktree/a",
	})
	createResp, _ := http.Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Now GET that session.
	resp, err := http.Get(ts.URL + "/api/sessions/" + created.ID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandleAbortSession_ValidSession(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Create a session.
	body, _ := json.Marshal(map[string]string{"worktreePath": "/worktree/a"})
	createResp, _ := http.Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()

	// Abort it.
	req, _ := http.NewRequest("POST", ts.URL+"/api/sessions/"+created.ID+"/abort", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST abort: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

// --- anyAdapter pattern ---

func TestHandleListAgents_NoInstances(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestHandleListAgents_WithInstance(t *testing.T) {
	srv, p, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	p.NewAdapterFunc = func(baseURL string) agent.Adapter {
		a := agenttest.NewMockAdapter()
		a.Agents = []agent.AgentInfo{
			{Name: "build", Description: "code agent"},
		}
		return a
	}

	// Spawn an instance by listing sessions for a worktree.
	http.Get(ts.URL + "/api/sessions?worktree=/worktree/a")

	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var agents []agent.AgentInfo
	json.NewDecoder(resp.Body).Decode(&agents)
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

// --- Instance management ---

func TestHandleListInstances(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// No instances initially.
	resp, _ := http.Get(ts.URL + "/api/instances")
	var instances []json.RawMessage
	json.NewDecoder(resp.Body).Decode(&instances)
	resp.Body.Close()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}

	// Spawn one.
	http.Get(ts.URL + "/api/sessions?worktree=/worktree/a")

	resp, _ = http.Get(ts.URL + "/api/instances")
	json.NewDecoder(resp.Body).Decode(&instances)
	resp.Body.Close()
	if len(instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(instances))
	}
}

func TestHandleStopInstance(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Spawn one.
	http.Get(ts.URL + "/api/sessions?worktree=/worktree/a")

	// Stop it.
	body, _ := json.Marshal(map[string]string{"workDir": "/worktree/a"})
	resp, err := http.Post(ts.URL+"/api/instances/stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Verify gone.
	resp2, _ := http.Get(ts.URL + "/api/instances")
	var instances []json.RawMessage
	json.NewDecoder(resp2.Body).Decode(&instances)
	resp2.Body.Close()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances after stop, got %d", len(instances))
	}
}

func TestHandleStopInstance_NotFound(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"workDir": "/worktree/nonexistent"})
	resp, err := http.Post(ts.URL+"/api/instances/stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
