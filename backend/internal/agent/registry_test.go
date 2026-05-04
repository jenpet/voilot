package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/agent/agenttest"
)

const testProvider = "mock"

// providerMap wraps a single mock provider in the map format required by NewProviderRegistry.
func providerMap(p *agenttest.MockProvider) map[string]agent.Provider {
	return map[string]agent.Provider{testProvider: p}
}

// newTestRegistry creates a ProviderRegistry with a MockProvider in a temp dir.
func newTestRegistry(t *testing.T, provider *agenttest.MockProvider, opts ...agent.RegistryOption) *agent.ProviderRegistry {
	t.Helper()
	pidDir := t.TempDir()
	reg, err := agent.NewProviderRegistry(providerMap(provider), testProvider, pidDir, opts...)
	if err != nil {
		t.Fatalf("NewProviderRegistry: %v", err)
	}
	t.Cleanup(func() { reg.Close() })
	return reg
}

// --- GetOrSpawn lifecycle ---

func TestGetOrSpawn_NewInstance(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	adapter, err := reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	if err != nil {
		t.Fatalf("GetOrSpawn: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if inst := reg.GetInstance("/worktree/a", testProvider); inst == nil {
		t.Fatal("expected instance to be registered")
	}
}

func TestGetOrSpawn_SameWorktreeReturnsExisting(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	a1, _ := reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	a2, _ := reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)

	// Should be the exact same adapter (pointer equality).
	if a1 != a2 {
		t.Error("expected same adapter for same worktree, got different")
	}
	if len(reg.ListInstances()) != 1 {
		t.Errorf("expected 1 instance, got %d", len(reg.ListInstances()))
	}
}

func TestGetOrSpawn_DifferentWorktreeSpawnsNew(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	a1, _ := reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	a2, _ := reg.GetOrSpawn(context.Background(), "/worktree/b", testProvider)

	if a1 == a2 {
		t.Error("expected different adapters for different worktrees")
	}
	if len(reg.ListInstances()) != 2 {
		t.Errorf("expected 2 instances, got %d", len(reg.ListInstances()))
	}
}

func TestGetOrSpawn_SpawnFailure(t *testing.T) {
	p := agenttest.NewMockProvider()
	p.SpawnFunc = func(_ context.Context, _ string) (string, int, error) {
		return "", 0, fmt.Errorf("binary not found")
	}
	reg := newTestRegistry(t, p)

	_, err := reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	if err == nil {
		t.Fatal("expected error from spawn failure")
	}
	if len(reg.ListInstances()) != 0 {
		t.Error("expected no instances after spawn failure")
	}
}

// --- LRU eviction ---

func TestEviction_LRUIdle(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p, agent.WithMaxInstances(2))

	// Spawn A and B.
	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	time.Sleep(10 * time.Millisecond)
	reg.GetOrSpawn(context.Background(), "/worktree/b", testProvider)

	// Touch A so B is the LRU.
	time.Sleep(10 * time.Millisecond)
	reg.TouchActivity("/worktree/a", testProvider)

	// Spawn C — should evict B.
	_, err := reg.GetOrSpawn(context.Background(), "/worktree/c", testProvider)
	if err != nil {
		t.Fatalf("GetOrSpawn /c: %v", err)
	}

	if reg.GetInstance("/worktree/b", testProvider) != nil {
		t.Error("expected /worktree/b to be evicted")
	}
	if reg.GetInstance("/worktree/a", testProvider) == nil {
		t.Error("expected /worktree/a to survive")
	}
	if reg.GetInstance("/worktree/c", testProvider) == nil {
		t.Error("expected /worktree/c to exist")
	}
}

func TestEviction_AllBusy(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p, agent.WithMaxInstances(2))

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	reg.GetOrSpawn(context.Background(), "/worktree/b", testProvider)

	// Mark both busy.
	reg.MarkBusy("/worktree/a", testProvider)
	reg.MarkBusy("/worktree/b", testProvider)

	_, err := reg.GetOrSpawn(context.Background(), "/worktree/c", testProvider)
	if err == nil {
		t.Fatal("expected error when all instances are busy")
	}
}

// --- Idle reaping ---

func TestReapIdle_ReapsExpiredIdle(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p, agent.WithIdleTimeout(50*time.Millisecond))

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)

	// Immediately — should not reap.
	reg.ReapIdle()
	if reg.GetInstance("/worktree/a", testProvider) == nil {
		t.Fatal("instance reaped too early")
	}

	// Wait past idle timeout.
	time.Sleep(60 * time.Millisecond)
	reg.ReapIdle()
	if reg.GetInstance("/worktree/a", testProvider) != nil {
		t.Fatal("expected instance to be reaped after idle timeout")
	}
}

func TestReapIdle_BusyInstanceSurvives(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p, agent.WithIdleTimeout(50*time.Millisecond))

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	reg.MarkBusy("/worktree/a", testProvider)

	time.Sleep(60 * time.Millisecond)
	reg.ReapIdle()
	if reg.GetInstance("/worktree/a", testProvider) == nil {
		t.Fatal("busy instance should not be reaped")
	}
}

// --- Activity tracking ---

func TestActivityTracking(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	inst := reg.GetInstance("/worktree/a", testProvider)

	if !inst.IsIdle() {
		t.Error("new instance should be idle")
	}

	reg.MarkBusy("/worktree/a", testProvider)
	if inst.IsIdle() {
		t.Error("instance should be busy after MarkBusy")
	}

	reg.MarkIdle("/worktree/a", testProvider)
	if !inst.IsIdle() {
		t.Error("instance should be idle after MarkIdle")
	}

	before := inst.LastActivity
	time.Sleep(5 * time.Millisecond)
	reg.TouchActivity("/worktree/a", testProvider)
	if !inst.LastActivity.After(before) {
		t.Error("TouchActivity should update LastActivity")
	}
}

// --- StopInstance ---

func TestStopInstance(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	if err := reg.StopInstance("/worktree/a", testProvider); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if reg.GetInstance("/worktree/a", testProvider) != nil {
		t.Error("instance should be removed after stop")
	}
}

func TestStopInstance_BusyReturnsError(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	reg.MarkBusy("/worktree/a", testProvider)

	if err := reg.StopInstance("/worktree/a", testProvider); err == nil {
		t.Error("expected error when stopping busy instance")
	}
}

func TestStopInstance_NotFound(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	if err := reg.StopInstance("/worktree/nonexistent", testProvider); err == nil {
		t.Error("expected error for nonexistent instance")
	}
}

// --- PID file orphan cleanup ---

func TestSweepOrphans_KillsLiveProcess(t *testing.T) {
	pidDir := t.TempDir()

	// Start a real process to orphan.
	cmd := exec.Command("sleep", "9999")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		// Best-effort kill in case test fails before sweep.
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Process.Release()
	})

	// Write a PID file as if a previous voilot run left it.
	entry := struct {
		PID     int    `json:"pid"`
		BaseURL string `json:"baseURL"`
		WorkDir string `json:"workDir"`
	}{
		PID:     pid,
		BaseURL: "http://127.0.0.1:9999",
		WorkDir: "/old/worktree",
	}
	data, _ := json.Marshal(entry)
	pidFile := filepath.Join(pidDir, "orphan.pid")
	if err := os.WriteFile(pidFile, data, 0o644); err != nil {
		t.Fatalf("write PID file: %v", err)
	}

	// Creating a new registry triggers sweepOrphans.
	p := agenttest.NewMockProvider()
	reg, err := agent.NewProviderRegistry(providerMap(p), testProvider, pidDir)
	if err != nil {
		t.Fatalf("NewProviderRegistry: %v", err)
	}
	defer reg.Close()

	// Verify PID file was removed.
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed by sweepOrphans")
	}

	// Verify process was killed — wait for it to exit.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// Process exited, good.
	case <-time.After(3 * time.Second):
		t.Error("expected orphaned process to be killed within 3s")
	}
}

func TestSweepOrphans_CleansUpStaleFile(t *testing.T) {
	pidDir := t.TempDir()

	// Write a PID file for a non-existent process (use a very high PID).
	entry := struct {
		PID     int    `json:"pid"`
		BaseURL string `json:"baseURL"`
		WorkDir string `json:"workDir"`
	}{
		PID:     999999999,
		BaseURL: "http://127.0.0.1:9998",
		WorkDir: "/dead/worktree",
	}
	data, _ := json.Marshal(entry)
	pidFile := filepath.Join(pidDir, "stale.pid")
	os.WriteFile(pidFile, data, 0o644)

	p := agenttest.NewMockProvider()
	reg, err := agent.NewProviderRegistry(providerMap(p), testProvider, pidDir)
	if err != nil {
		t.Fatalf("NewProviderRegistry: %v", err)
	}
	defer reg.Close()

	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("expected stale PID file to be cleaned up")
	}
}

// --- PID file written on spawn, removed on stop ---

func TestPIDFile_WrittenAndRemoved(t *testing.T) {
	pidDir := t.TempDir()
	p := agenttest.NewMockProvider()
	reg, err := agent.NewProviderRegistry(providerMap(p), testProvider, pidDir)
	if err != nil {
		t.Fatalf("NewProviderRegistry: %v", err)
	}
	defer reg.Close()

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)

	// Check PID file exists.
	entries, _ := os.ReadDir(pidDir)
	pidFiles := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".pid" {
			pidFiles++
		}
	}
	if pidFiles != 1 {
		t.Errorf("expected 1 PID file, got %d", pidFiles)
	}

	// Stop and verify removed.
	reg.StopInstance("/worktree/a", testProvider)
	entries, _ = os.ReadDir(pidDir)
	pidFiles = 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".pid" {
			pidFiles++
		}
	}
	if pidFiles != 0 {
		t.Errorf("expected 0 PID files after stop, got %d", pidFiles)
	}
}

// --- SSE aggregation ---

func TestSSEAggregation(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	// Spawn two instances.
	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	reg.GetOrSpawn(context.Background(), "/worktree/b", testProvider)

	// Subscribe to aggregated events.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := reg.SubscribeEvents(ctx)

	// Give aggregateSSE goroutines time to subscribe.
	time.Sleep(50 * time.Millisecond)

	// Push events into each mock adapter's channel.
	instA := reg.GetInstance("/worktree/a", testProvider)
	instB := reg.GetInstance("/worktree/b", testProvider)
	adapterA := instA.Adapter.(*agenttest.MockAdapter)
	adapterB := instB.Adapter.(*agenttest.MockAdapter)

	adapterA.EventCh <- agent.Event{Type: agent.EventText, SessionID: "s1", Content: "from-a"}
	adapterB.EventCh <- agent.Event{Type: agent.EventText, SessionID: "s2", Content: "from-b"}

	// Collect events with timeout.
	received := map[string]bool{}
	timeout := time.After(2 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case evt := <-ch:
			received[evt.Content] = true
		case <-timeout:
			t.Fatalf("timed out waiting for events, got %d", len(received))
		}
	}
	if !received["from-a"] || !received["from-b"] {
		t.Errorf("expected events from both adapters, got %v", received)
	}
}

// --- Concurrent GetOrSpawn ---

func TestGetOrSpawn_ConcurrentSameWorktree(t *testing.T) {
	p := agenttest.NewMockProvider()
	var spawnCount int32
	origSpawn := p.SpawnFunc
	p.SpawnFunc = func(ctx context.Context, workDir string) (string, int, error) {
		atomic.AddInt32(&spawnCount, 1)
		// Add a small delay to increase chance of race.
		time.Sleep(10 * time.Millisecond)
		if origSpawn != nil {
			return origSpawn(ctx, workDir)
		}
		return fmt.Sprintf("http://127.0.0.1:%d", 20000+atomic.LoadInt32(&spawnCount)), int(atomic.LoadInt32(&spawnCount)), nil
	}
	reg := newTestRegistry(t, p)

	var wg sync.WaitGroup
	adapters := make([]agent.Adapter, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			a, err := reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
				return
			}
			adapters[idx] = a
		}(i)
	}
	wg.Wait()

	// Should have spawned exactly once.
	if c := atomic.LoadInt32(&spawnCount); c != 1 {
		t.Errorf("expected 1 spawn, got %d", c)
	}

	// All adapters should be the same.
	for i := 1; i < 10; i++ {
		if adapters[i] != adapters[0] {
			t.Error("expected all goroutines to get the same adapter")
			break
		}
	}
}

// --- Close ---

func TestClose_StopsAllInstances(t *testing.T) {
	p := agenttest.NewMockProvider()
	pidDir := t.TempDir()
	reg, err := agent.NewProviderRegistry(providerMap(p), testProvider, pidDir)
	if err != nil {
		t.Fatalf("NewProviderRegistry: %v", err)
	}

	reg.GetOrSpawn(context.Background(), "/worktree/a", testProvider)
	reg.GetOrSpawn(context.Background(), "/worktree/b", testProvider)

	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if p.StoppedCount() != 2 {
		t.Errorf("expected 2 stopped instances, got %d", p.StoppedCount())
	}
}

// --- Symlink resolution ---

func TestGetOrSpawn_SymlinkResolvesToSameInstance(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	// Create a real directory and a symlink pointing to it.
	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Spawn via the real path.
	a1, err := reg.GetOrSpawn(context.Background(), realDir, testProvider)
	if err != nil {
		t.Fatalf("GetOrSpawn(real): %v", err)
	}

	// Spawn via the symlink — should return the same instance, not a new one.
	a2, err := reg.GetOrSpawn(context.Background(), symlinkDir, testProvider)
	if err != nil {
		t.Fatalf("GetOrSpawn(symlink): %v", err)
	}

	if a1 != a2 {
		t.Error("expected symlink and real path to resolve to the same adapter instance")
	}
	if reg.InstanceCount() != 1 {
		t.Errorf("expected 1 instance, got %d", reg.InstanceCount())
	}
}

func TestGetInstance_ViaSymlink(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Spawn via symlink, look up via real path.
	if _, err := reg.GetOrSpawn(context.Background(), symlinkDir, testProvider); err != nil {
		t.Fatalf("GetOrSpawn: %v", err)
	}

	inst := reg.GetInstance(realDir, testProvider)
	if inst == nil {
		t.Error("expected to find instance via real path after spawning with symlink")
	}

	// And vice versa — look up via symlink.
	inst2 := reg.GetInstance(symlinkDir, testProvider)
	if inst2 == nil {
		t.Error("expected to find instance via symlink path")
	}
	if inst != inst2 {
		t.Error("expected same instance from both lookups")
	}
}

func TestStopInstance_ViaSymlink(t *testing.T) {
	p := agenttest.NewMockProvider()
	reg := newTestRegistry(t, p)

	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Spawn via real path, stop via symlink.
	if _, err := reg.GetOrSpawn(context.Background(), realDir, testProvider); err != nil {
		t.Fatalf("GetOrSpawn: %v", err)
	}
	if err := reg.StopInstance(symlinkDir, testProvider); err != nil {
		t.Fatalf("StopInstance via symlink: %v", err)
	}
	if reg.InstanceCount() != 0 {
		t.Errorf("expected 0 instances after stop, got %d", reg.InstanceCount())
	}
}
