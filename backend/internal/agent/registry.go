package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// DefaultMaxInstances is the default maximum number of concurrent provider instances.
const DefaultMaxInstances = 5

// DefaultIdleTimeout is the default time after which an idle instance is torn down.
const DefaultIdleTimeout = 10 * time.Minute

// Instance represents a running agent backend process managed by the registry.
type Instance struct {
	WorkDir      string
	ProviderName string
	BaseURL      string
	Adapter      Adapter
	PID          int
	LastActivity time.Time
	activeCount  int32 // atomic: number of sessions with active work
}

// resolveWorktreePath resolves symlinks in a worktree path to its canonical form.
// Falls back to the original path if resolution fails.
func resolveWorktreePath(p string) string {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return resolved
}

// instanceKey builds the composite map key for (worktreePath, providerName).
func instanceKey(worktreePath, providerName string) string {
	return providerName + "\x00" + worktreePath
}

// IsIdle returns true if the instance has no active sessions.
func (inst *Instance) IsIdle() bool {
	return atomic.LoadInt32(&inst.activeCount) == 0
}

// pidFileEntry is the JSON structure stored in PID files.
type pidFileEntry struct {
	PID     int    `json:"pid"`
	Port    int    `json:"port,omitempty"`
	BaseURL string `json:"baseURL"`
	WorkDir string `json:"workDir"`
}

// ProviderRegistry manages provider instances per worktree.
// It handles spawning, port tracking, health checks, idle teardown,
// and PID file management for orphan cleanup.
// Supports multiple providers keyed by name; instances are keyed by
// (worktreePath, providerName) composite.
type ProviderRegistry struct {
	providers       map[string]Provider // name -> provider
	defaultProvider string
	mu              sync.RWMutex
	instances       map[string]*Instance // instanceKey -> instance
	maxInstances    int
	idleTimeout     time.Duration
	pidDir          string // directory for PID files

	// SSE aggregation: single channel that fans out events from all adapters
	sseSubMu       sync.RWMutex
	sseSubscribers map[chan Event]struct{}

	// Shutdown coordination
	cancel  context.CancelFunc
	stopped chan struct{}
}

// RegistryOption configures the ProviderRegistry.
type RegistryOption func(*ProviderRegistry)

// WithMaxInstances sets the maximum number of concurrent instances.
func WithMaxInstances(n int) RegistryOption {
	return func(r *ProviderRegistry) { r.maxInstances = n }
}

// WithIdleTimeout sets the idle timeout for instance teardown.
func WithIdleTimeout(d time.Duration) RegistryOption {
	return func(r *ProviderRegistry) { r.idleTimeout = d }
}

// NewProviderRegistry creates a new registry for the given providers.
// defaultProvider must be a key in the providers map.
// pidDir is the directory where PID files are stored for orphan cleanup.
func NewProviderRegistry(providers map[string]Provider, defaultProvider string, pidDir string, opts ...RegistryOption) (*ProviderRegistry, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}
	if _, ok := providers[defaultProvider]; !ok {
		return nil, fmt.Errorf("default provider %q not found in providers map", defaultProvider)
	}
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return nil, fmt.Errorf("create PID directory %s: %w", pidDir, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &ProviderRegistry{
		providers:       providers,
		defaultProvider: defaultProvider,
		instances:       make(map[string]*Instance),
		maxInstances:    DefaultMaxInstances,
		idleTimeout:     DefaultIdleTimeout,
		pidDir:          pidDir,
		sseSubscribers:  make(map[chan Event]struct{}),
		cancel:          cancel,
		stopped:         make(chan struct{}),
	}
	for _, opt := range opts {
		opt(r)
	}

	// Sweep orphaned PID files from previous runs
	r.sweepOrphans()

	// Start idle reaper
	go r.idleReaper(ctx)

	return r, nil
}

// DefaultProviderName returns the name of the default provider.
func (r *ProviderRegistry) DefaultProviderName() string {
	return r.defaultProvider
}

// ProviderNames returns the names of all configured providers.
func (r *ProviderRegistry) ProviderNames() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetOrSpawn returns the adapter for the given worktree path and provider,
// spawning a new instance if one doesn't already exist. If the max instance
// limit is reached, the least recently active idle instance is evicted.
func (r *ProviderRegistry) GetOrSpawn(ctx context.Context, worktreePath string, providerName string) (Adapter, error) {
	worktreePath = resolveWorktreePath(worktreePath)
	provider, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", providerName)
	}

	key := instanceKey(worktreePath, providerName)

	r.mu.RLock()
	if inst, ok := r.instances[key]; ok {
		inst.LastActivity = time.Now()
		r.mu.RUnlock()
		return inst.Adapter, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if inst, ok := r.instances[key]; ok {
		inst.LastActivity = time.Now()
		return inst.Adapter, nil
	}

	// Evict if at capacity
	if len(r.instances) >= r.maxInstances {
		if err := r.evictLRUIdle(); err != nil {
			return nil, fmt.Errorf("all %d instance slots occupied with active sessions", r.maxInstances)
		}
	}

	// Spawn new instance
	baseURL, pid, err := provider.Spawn(ctx, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("spawn %s in %s: %w", providerName, worktreePath, err)
	}

	adapter := provider.NewAdapter(baseURL)
	inst := &Instance{
		WorkDir:      worktreePath,
		ProviderName: providerName,
		BaseURL:      baseURL,
		Adapter:      adapter,
		PID:          pid,
		LastActivity: time.Now(),
	}
	r.instances[key] = inst

	// Write PID file
	r.writePIDFile(key, inst)

	// Subscribe to this adapter's SSE events and aggregate
	go r.aggregateSSE(key, adapter)

	return adapter, nil
}

// TouchActivity updates the last activity timestamp for a worktree's instance.
func (r *ProviderRegistry) TouchActivity(worktreePath, providerName string) {
	worktreePath = resolveWorktreePath(worktreePath)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if inst, ok := r.instances[instanceKey(worktreePath, providerName)]; ok {
		inst.LastActivity = time.Now()
	}
}

// MarkBusy increments the active session count for a worktree's instance.
func (r *ProviderRegistry) MarkBusy(worktreePath, providerName string) {
	worktreePath = resolveWorktreePath(worktreePath)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if inst, ok := r.instances[instanceKey(worktreePath, providerName)]; ok {
		atomic.AddInt32(&inst.activeCount, 1)
		inst.LastActivity = time.Now()
	}
}

// MarkIdle decrements the active session count for a worktree's instance.
func (r *ProviderRegistry) MarkIdle(worktreePath, providerName string) {
	worktreePath = resolveWorktreePath(worktreePath)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if inst, ok := r.instances[instanceKey(worktreePath, providerName)]; ok {
		if v := atomic.AddInt32(&inst.activeCount, -1); v < 0 {
			atomic.StoreInt32(&inst.activeCount, 0)
		}
		inst.LastActivity = time.Now()
	}
}

// StopInstance explicitly stops an instance for the given worktree and provider.
// Returns an error if the instance is currently busy.
func (r *ProviderRegistry) StopInstance(worktreePath, providerName string) error {
	worktreePath = resolveWorktreePath(worktreePath)
	r.mu.Lock()
	defer r.mu.Unlock()

	key := instanceKey(worktreePath, providerName)
	inst, ok := r.instances[key]
	if !ok {
		return fmt.Errorf("no instance for worktree %s (provider %s)", worktreePath, providerName)
	}
	if !inst.IsIdle() {
		return fmt.Errorf("instance for %s (provider %s) is currently busy", worktreePath, providerName)
	}
	return r.stopInstanceLocked(key, inst)
}

// GetInstance returns the instance for a worktree and provider, or nil if not running.
func (r *ProviderRegistry) GetInstance(worktreePath, providerName string) *Instance {
	worktreePath = resolveWorktreePath(worktreePath)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.instances[instanceKey(worktreePath, providerName)]
}

// ListInstances returns all running instances.
func (r *ProviderRegistry) ListInstances() []*Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Instance, 0, len(r.instances))
	for _, inst := range r.instances {
		result = append(result, inst)
	}
	return result
}

// InstanceCount returns the number of running instances.
func (r *ProviderRegistry) InstanceCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.instances)
}

// Ready checks whether all configured providers can spawn new instances.
func (r *ProviderRegistry) Ready(ctx context.Context) error {
	for name, p := range r.providers {
		if err := p.Ready(ctx); err != nil {
			return fmt.Errorf("provider %s: %w", name, err)
		}
	}
	return nil
}

// SubscribeEvents returns a channel that receives aggregated SSE events
// from all active adapter instances. The channel is closed when the
// context is cancelled.
func (r *ProviderRegistry) SubscribeEvents(ctx context.Context) <-chan Event {
	ch := make(chan Event, 64)

	r.sseSubMu.Lock()
	r.sseSubscribers[ch] = struct{}{}
	r.sseSubMu.Unlock()

	go func() {
		<-ctx.Done()
		r.sseSubMu.Lock()
		delete(r.sseSubscribers, ch)
		r.sseSubMu.Unlock()
		close(ch)
	}()

	return ch
}

// Close stops all managed instances and cleans up resources.
func (r *ProviderRegistry) Close() error {
	r.cancel()
	<-r.stopped

	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for path, inst := range r.instances {
		if err := r.stopInstanceLocked(path, inst); err != nil {
			log.Printf("Failed to stop instance %s: %v", path, err)
			lastErr = err
		}
	}
	return lastErr
}

// --- Internal methods ---

// stopInstanceLocked stops an instance and removes it from the registry.
// Caller must hold r.mu write lock.
func (r *ProviderRegistry) stopInstanceLocked(key string, inst *Instance) error {
	provider, ok := r.providers[inst.ProviderName]
	if !ok {
		return fmt.Errorf("provider %q not found for instance %s", inst.ProviderName, key)
	}
	err := provider.Stop(inst.PID)
	delete(r.instances, key)
	r.removePIDFile(key)
	log.Printf("Stopped instance for %s [%s] (pid=%d)", inst.WorkDir, inst.ProviderName, inst.PID)
	return err
}

// evictLRUIdle finds and stops the least recently active idle instance.
// Caller must hold r.mu write lock. Returns an error if no idle instance exists.
func (r *ProviderRegistry) evictLRUIdle() error {
	var oldest *Instance
	var oldestPath string

	for path, inst := range r.instances {
		if inst.IsIdle() && (oldest == nil || inst.LastActivity.Before(oldest.LastActivity)) {
			oldest = inst
			oldestPath = path
		}
	}

	if oldest == nil {
		return fmt.Errorf("no idle instances to evict")
	}

	log.Printf("Evicting LRU idle instance: %s (last active %s)", oldestPath, oldest.LastActivity.Format(time.RFC3339))
	return r.stopInstanceLocked(oldestPath, oldest)
}

// idleReaper periodically checks for idle instances and tears them down.
func (r *ProviderRegistry) idleReaper(ctx context.Context) {
	defer close(r.stopped)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.ReapIdle()
		}
	}
}

// ReapIdle tears down instances that are idle and past the timeout.
// Exported for testing; the background reaper calls this periodically.
func (r *ProviderRegistry) ReapIdle() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	// Collect paths to stop (can't modify map while iterating)
	var toStop []string
	for path, inst := range r.instances {
		if inst.IsIdle() && now.Sub(inst.LastActivity) > r.idleTimeout {
			toStop = append(toStop, path)
		}
	}

	for _, path := range toStop {
		inst := r.instances[path]
		log.Printf("Reaping idle instance: %s (idle for %s)", path, now.Sub(inst.LastActivity).Round(time.Second))
		if err := r.stopInstanceLocked(path, inst); err != nil {
			log.Printf("Failed to reap instance %s: %v", path, err)
		}
	}
}

// aggregateSSE subscribes to an adapter's SSE events and forwards them
// to all registry-level subscribers. Exits when the adapter disconnects
// or the instance is removed.
func (r *ProviderRegistry) aggregateSSE(worktreePath string, adapter Adapter) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, err := adapter.SubscribeEvents(ctx)
	if err != nil {
		log.Printf("Failed to subscribe to SSE for %s: %v", worktreePath, err)
		return
	}

	for evt := range eventCh {
		r.sseSubMu.RLock()
		for ch := range r.sseSubscribers {
			select {
			case ch <- evt:
			default:
				// Slow subscriber, drop event
			}
		}
		r.sseSubMu.RUnlock()
	}
}

// --- PID file management ---

func (r *ProviderRegistry) pidFilePath(worktreePath string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(worktreePath)))
	return filepath.Join(r.pidDir, hash[:16]+".pid")
}

func (r *ProviderRegistry) writePIDFile(worktreePath string, inst *Instance) {
	entry := pidFileEntry{
		PID:     inst.PID,
		BaseURL: inst.BaseURL,
		WorkDir: worktreePath,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal PID file for %s: %v", worktreePath, err)
		return
	}
	path := r.pidFilePath(worktreePath)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("Failed to write PID file %s: %v", path, err)
	}
}

func (r *ProviderRegistry) removePIDFile(worktreePath string) {
	path := r.pidFilePath(worktreePath)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to remove PID file %s: %v", path, err)
	}
}

// sweepOrphans kills any processes from PID files left by a previous crash.
func (r *ProviderRegistry) sweepOrphans() {
	entries, err := os.ReadDir(r.pidDir)
	if err != nil {
		log.Printf("Failed to read PID directory %s: %v", r.pidDir, err)
		return
	}

	// Sort for deterministic logging
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".pid" {
			continue
		}

		path := filepath.Join(r.pidDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Failed to read PID file %s: %v", path, err)
			_ = os.Remove(path)
			continue
		}

		var pf pidFileEntry
		if err := json.Unmarshal(data, &pf); err != nil {
			log.Printf("Failed to parse PID file %s: %v", path, err)
			_ = os.Remove(path)
			continue
		}

		// Check if process is still running
		proc, err := os.FindProcess(pf.PID)
		if err != nil {
			_ = os.Remove(path)
			continue
		}

		// On Unix, FindProcess always succeeds. Use kill -0 to check.
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process is dead, clean up PID file
			log.Printf("Cleaned up stale PID file: %s (pid %d, dir %s)", path, pf.PID, pf.WorkDir)
			_ = os.Remove(path)
			continue
		}

		// Process is alive — kill it
		log.Printf("Killing orphaned process: pid %d (dir %s)", pf.PID, pf.WorkDir)
		_ = proc.Signal(syscall.SIGTERM)
		_ = os.Remove(path)
	}
}
