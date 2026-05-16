package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jenpet/voilot/internal/agent"
)

// stubProvider is a minimal Provider implementation for testing.
type stubProvider struct{}

func (p *stubProvider) Name() string                                           { return "stub" }
func (p *stubProvider) Ready(_ context.Context) error                          { return nil }
func (p *stubProvider) Spawn(_ context.Context, _ string) (string, int, error) { return "", 0, nil }
func (p *stubProvider) Healthy(_ context.Context, _ string) bool               { return true }
func (p *stubProvider) Stop(_ int) error                                       { return nil }
func (p *stubProvider) NewAdapter(_ string) agent.Adapter                      { return nil }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	providers := map[string]agent.Provider{"stub": &stubProvider{}}
	registry, err := agent.NewProviderRegistry(providers, "stub", t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	return NewServer(registry, nil, nil, nil, nil, nil, BuildInfo{Version: "test", BuildTime: "now"})
}

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}
