package agent

import "context"

// Provider knows how to spawn, health-check, and stop an agent backend process.
// Each provider implementation manages a specific agent backend (e.g. OpenCode).
type Provider interface {
	// Name returns the provider identifier (e.g. "opencode").
	Name() string

	// Ready checks whether the provider can spawn new instances
	// (e.g. binary exists and is executable). Returns nil if ready.
	Ready(ctx context.Context) error

	// Spawn starts a new instance in the given working directory.
	// Returns the base URL and OS process ID of the running instance.
	// The implementation must wait for the instance to become healthy
	// before returning.
	Spawn(ctx context.Context, workDir string) (baseURL string, pid int, err error)

	// Healthy returns true if the instance at baseURL is responsive.
	Healthy(ctx context.Context, baseURL string) bool

	// Stop terminates the instance identified by its OS process ID.
	Stop(pid int) error

	// NewAdapter creates an Adapter connected to the given baseURL.
	NewAdapter(baseURL string) Adapter
}
