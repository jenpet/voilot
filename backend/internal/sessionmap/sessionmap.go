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

// Entry holds all voilot-side metadata for a session.
type Entry struct {
	WorktreePath string `json:"worktreePath,omitempty"`
	Title        string `json:"title,omitempty"`
	UpdatedAt    int64  `json:"updatedAt,omitempty"` // last activity timestamp (epoch ms), from backfill or runtime
}

// Map manages session-to-entry mappings with file-backed persistence.
type Map struct {
	mu       sync.RWMutex
	filePath string
	entries  map[string]Entry // sessionID -> Entry
}

// New creates or loads a session map from the given file path.
// If the file does not exist, an empty map is created.
// Supports migration from the legacy format (map[string]string).
func New(filePath string) (*Map, error) {
	m := &Map{
		filePath: filePath,
		entries:  make(map[string]Entry),
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

	// Try new format first: map[string]Entry
	if err := json.Unmarshal(data, &m.entries); err == nil {
		return m, nil
	}

	// Fall back to legacy format: map[string]string (sessionID -> worktreePath)
	legacy := make(map[string]string)
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse session map: %w", err)
	}
	for id, path := range legacy {
		m.entries[id] = Entry{WorktreePath: path}
	}
	// Persist in new format immediately
	if err := m.save(); err != nil {
		return nil, fmt.Errorf("migrate session map: %w", err)
	}
	return m, nil
}

// Set associates a session ID with a worktree path and persists.
func (m *Map) Set(sessionID, worktreePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[sessionID]
	e.WorktreePath = worktreePath
	m.entries[sessionID] = e
	return m.save()
}

// Get returns the worktree path for a session, or empty string if not mapped.
func (m *Map) Get(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries[sessionID].WorktreePath
}

// GetEntry returns the full entry for a session.
func (m *Map) GetEntry(sessionID string) Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries[sessionID]
}

// SetTitle stores a manual title override for a session and persists.
func (m *Map) SetTitle(sessionID, title string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[sessionID]
	e.Title = title
	m.entries[sessionID] = e
	return m.save()
}

// GetTitle returns the manual title override for a session, or empty string.
func (m *Map) GetTitle(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.entries[sessionID].Title
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
	for id, e := range m.entries {
		if e.WorktreePath == worktreePath {
			ids = append(ids, id)
		}
	}
	return ids
}

// LastActivityForWorktree returns the most recent UpdatedAt timestamp
// across all sessions mapped to the given worktree path.
func (m *Map) LastActivityForWorktree(worktreePath string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var max int64
	for _, e := range m.entries {
		if e.WorktreePath == worktreePath && e.UpdatedAt > max {
			max = e.UpdatedAt
		}
	}
	return max
}

// SetEntry stores a full entry for a session and persists.
func (m *Map) SetEntry(sessionID string, e Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[sessionID] = e
	return m.save()
}

// UpdateTimestamp updates the UpdatedAt field for a session without
// touching other fields. No-op if session is not in the map.
func (m *Map) UpdateTimestamp(sessionID string, ts int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[sessionID]
	if !ok {
		return
	}
	if ts > e.UpdatedAt {
		e.UpdatedAt = ts
		m.entries[sessionID] = e
		// Best-effort save; don't block on timestamp updates.
		_ = m.save()
	}
}

// AllEntries returns a copy of all mappings.
func (m *Map) AllEntries() map[string]Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[string]Entry, len(m.entries))
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
