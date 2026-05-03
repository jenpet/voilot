package agenttest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/jenpet/voilot/internal/agent"
)

// MockProvider is an in-memory fake implementing agent.Provider for tests.
type MockProvider struct {
	mu        sync.Mutex
	instances map[string]mockInstance // workDir -> instance
	nextPID   int32

	// SpawnFunc overrides the default spawn behavior. Return an error to simulate spawn failures.
	SpawnFunc func(ctx context.Context, workDir string) (string, int, error)

	// HealthFunc overrides the default health check. Return false to simulate unhealthy instances.
	HealthFunc func(ctx context.Context, baseURL string) bool

	// NewAdapterFunc overrides the default adapter creation.
	// If nil, a new MockAdapter is created for each spawn.
	NewAdapterFunc func(baseURL string) agent.Adapter

	// AdaptersByURL tracks adapters created, keyed by baseURL.
	adaptersByURL map[string]*MockAdapter
	adapterMu     sync.Mutex
}

type mockInstance struct {
	baseURL string
	pid     int
	stopped bool
}

// Verify interface compliance.
var _ agent.Provider = (*MockProvider)(nil)

// NewMockProvider creates a new MockProvider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		instances:     make(map[string]mockInstance),
		adaptersByURL: make(map[string]*MockAdapter),
	}
}

func (p *MockProvider) Name() string { return "mock" }

func (p *MockProvider) Ready(_ context.Context) error { return nil }

func (p *MockProvider) Spawn(ctx context.Context, workDir string) (string, int, error) {
	if p.SpawnFunc != nil {
		return p.SpawnFunc(ctx, workDir)
	}

	pid := int(atomic.AddInt32(&p.nextPID, 1))
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", 10000+pid)

	p.mu.Lock()
	p.instances[workDir] = mockInstance{baseURL: baseURL, pid: pid}
	p.mu.Unlock()

	return baseURL, pid, nil
}

func (p *MockProvider) Healthy(ctx context.Context, baseURL string) bool {
	if p.HealthFunc != nil {
		return p.HealthFunc(ctx, baseURL)
	}
	return true
}

func (p *MockProvider) Stop(pid int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.instances {
		if v.pid == pid {
			v.stopped = true
			p.instances[k] = v
			return nil
		}
	}
	return fmt.Errorf("no instance with pid %d", pid)
}

func (p *MockProvider) NewAdapter(baseURL string) agent.Adapter {
	if p.NewAdapterFunc != nil {
		return p.NewAdapterFunc(baseURL)
	}
	a := NewMockAdapter()
	p.adapterMu.Lock()
	p.adaptersByURL[baseURL] = a
	p.adapterMu.Unlock()
	return a
}

// GetAdapter returns the MockAdapter created for a given baseURL, if any.
func (p *MockProvider) GetAdapter(baseURL string) *MockAdapter {
	p.adapterMu.Lock()
	defer p.adapterMu.Unlock()
	return p.adaptersByURL[baseURL]
}

// IsStopped returns true if the instance with the given PID was stopped.
func (p *MockProvider) IsStopped(pid int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, v := range p.instances {
		if v.pid == pid {
			return v.stopped
		}
	}
	return false
}

// StoppedCount returns how many instances have been stopped.
func (p *MockProvider) StoppedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, v := range p.instances {
		if v.stopped {
			n++
		}
	}
	return n
}
