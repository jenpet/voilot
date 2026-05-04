// Package agent — WorktreeDefaults manages per-worktree default provider preferences.
// Stored as runtime state in <data-dir>/worktree-defaults.json.
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// WorktreeDefaults manages per-worktree default provider preferences.
// The data is stored as a JSON file and is runtime state (not config).
type WorktreeDefaults struct {
	mu       sync.RWMutex
	filePath string
	defaults map[string]string // worktreePath -> providerName
}

// NewWorktreeDefaults creates or loads worktree defaults from the given file.
func NewWorktreeDefaults(filePath string) (*WorktreeDefaults, error) {
	wd := &WorktreeDefaults{
		filePath: filePath,
		defaults: make(map[string]string),
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, fmt.Errorf("create worktree defaults dir: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return wd, nil
		}
		return nil, fmt.Errorf("read worktree defaults: %w", err)
	}

	if err := json.Unmarshal(data, &wd.defaults); err != nil {
		return nil, fmt.Errorf("parse worktree defaults: %w", err)
	}

	return wd, nil
}

// Get returns the default provider for a worktree, or empty string if not set.
func (wd *WorktreeDefaults) Get(worktreePath string) string {
	wd.mu.RLock()
	defer wd.mu.RUnlock()
	return wd.defaults[worktreePath]
}

// Set stores the default provider for a worktree and persists.
func (wd *WorktreeDefaults) Set(worktreePath, providerName string) error {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	if providerName == "" {
		delete(wd.defaults, worktreePath)
	} else {
		wd.defaults[worktreePath] = providerName
	}
	return wd.save()
}

// All returns a copy of all worktree defaults.
func (wd *WorktreeDefaults) All() map[string]string {
	wd.mu.RLock()
	defer wd.mu.RUnlock()
	cp := make(map[string]string, len(wd.defaults))
	for k, v := range wd.defaults {
		cp[k] = v
	}
	return cp
}

func (wd *WorktreeDefaults) save() error {
	data, err := json.MarshalIndent(wd.defaults, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal worktree defaults: %w", err)
	}
	return os.WriteFile(wd.filePath, data, 0o644)
}
