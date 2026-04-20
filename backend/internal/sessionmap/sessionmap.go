// Package sessionmap persists the mapping between OpenCode session IDs
// and workspace worktree paths. The mapping is stored as a JSON file
// that survives backend restarts.
package sessionmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Map manages session-to-worktree mappings with file-backed persistence.
type Map struct {
	mu       sync.RWMutex
	filePath string
	entries  map[string]string // sessionID -> worktree absolute path
}

// New creates or loads a session map from the given file path.
// If the file does not exist, an empty map is created.
func New(filePath string) (*Map, error) {
	m := &Map{
		filePath: filePath,
		entries:  make(map[string]string),
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, fmt.Errorf("create session map dir: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("read session map: %w", err)
	}

	if err := json.Unmarshal(data, &m.entries); err != nil {
		return nil, fmt.Errorf("parse session map: %w", err)
	}
	return m, nil
}

// Set associates a session ID with a worktree path and persists.
func (m *Map) Set(sessionID, worktreePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[sessionID] = worktreePath
	return m.save()
}

// Get returns the worktree path for a session, or empty string if not mapped.
func (m *Map) Get(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries[sessionID]
}

// Delete removes a session mapping and persists.
func (m *Map) Delete(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, sessionID)
	return m.save()
}

// SessionsForWorktree returns all session IDs mapped to the given worktree path.
func (m *Map) SessionsForWorktree(worktreePath string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var ids []string
	for id, path := range m.entries {
		if path == worktreePath {
			ids = append(ids, id)
		}
	}
	return ids
}

// AllEntries returns a copy of all mappings.
func (m *Map) AllEntries() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[string]string, len(m.entries))
	for k, v := range m.entries {
		cp[k] = v
	}
	return cp
}

func (m *Map) save() error {
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session map: %w", err)
	}
	return os.WriteFile(m.filePath, data, 0o644)
}
