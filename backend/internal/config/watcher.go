package config

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// ConfigChange represents a detected config file change with both old and new configs.
type ConfigChange struct {
	Old *Config
	New *Config
}

// Watcher watches the config file for changes via mtime polling and emits
// validated configs on a channel when changes are detected.
type Watcher struct {
	path     string
	interval time.Duration
	changes  chan ConfigChange
}

// NewWatcher creates a new config file watcher.
// The interval specifies how frequently the file's mtime is polled.
func NewWatcher(path string, interval time.Duration) *Watcher {
	return &Watcher{
		path:     path,
		interval: interval,
		changes:  make(chan ConfigChange, 1),
	}
}

// Changes returns a read-only channel that emits ConfigChange values
// whenever the config file is modified and the new config is valid.
func (w *Watcher) Changes() <-chan ConfigChange {
	return w.changes
}

// Start begins polling the config file for changes. It blocks until the
// context is cancelled. The channel returned by Changes() is closed when
// Start returns.
func (w *Watcher) Start(ctx context.Context) {
	defer close(w.changes)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	var lastMod time.Time
	// Initialize lastMod to avoid triggering on first tick.
	if info, err := os.Stat(w.path); err == nil {
		lastMod = info.ModTime()
	}

	// Load the current config as the baseline for change comparison.
	current, _ := Load(w.path)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if err != nil {
				if os.IsNotExist(err) {
					slog.Error("Config file missing, keeping current config", "path", w.path)
				} else {
					slog.Error("Failed to stat config file, keeping current config", "path", w.path, "error", err)
				}
				continue
			}

			if info.ModTime().Equal(lastMod) {
				continue
			}
			lastMod = info.ModTime()

			newCfg, err := Load(w.path)
			if err != nil {
				slog.Error("Config reload failed, keeping current config", "path", w.path, "error", err)
				continue
			}

			old := current
			current = newCfg

			slog.Info("Config file changed, reloading", "path", w.path)

			select {
			case w.changes <- ConfigChange{Old: old, New: newCfg}:
			case <-ctx.Done():
				return
			}
		}
	}
}
