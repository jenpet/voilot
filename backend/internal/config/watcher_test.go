package config

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	// Write initial valid config
	initial := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode"}
		}
	}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	// Small poll interval for testing
	watcher := NewWatcher(path, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Start(ctx)

	// Wait a tick, then modify the file
	time.Sleep(100 * time.Millisecond)
	updated := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode", "env": {"MY_VAR": "hello"}}
		}
	}`
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for change to be detected
	select {
	case change := <-watcher.Changes():
		if change.New == nil {
			t.Fatal("expected non-nil new config")
		}
		env := change.New.Providers["opencode"].Env
		if env["MY_VAR"] != "hello" {
			t.Errorf("expected MY_VAR=hello, got %q", env["MY_VAR"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for config change")
	}
}

func TestWatcher_IgnoresUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	initial := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode"}
		}
	}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(path, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Start(ctx)

	// Wait several poll cycles without modifying the file
	select {
	case <-watcher.Changes():
		t.Fatal("expected no change, but got one")
	case <-time.After(300 * time.Millisecond):
		// Expected: no change emitted
	}
}

func TestWatcher_InvalidFileKeepsOldConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	initial := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode"}
		}
	}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(path, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Start(ctx)

	// Wait a tick, then write invalid JSON
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(path, []byte("invalid json{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should NOT emit a change (invalid config is rejected)
	select {
	case <-watcher.Changes():
		t.Fatal("expected no change for invalid config, but got one")
	case <-time.After(300 * time.Millisecond):
		// Expected: no change emitted
	}
}

func TestWatcher_DeletedFileKeepsOldConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	initial := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode"}
		}
	}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(path, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Start(ctx)

	// Wait a tick, then delete the file
	time.Sleep(100 * time.Millisecond)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Should NOT emit a change
	select {
	case <-watcher.Changes():
		t.Fatal("expected no change for deleted config, but got one")
	case <-time.After(300 * time.Millisecond):
		// Expected: no change emitted
	}
}

func TestWatcher_ClosesChannelOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	initial := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode"}
		}
	}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(path, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go watcher.Start(ctx)

	// Cancel context
	cancel()

	// Channel should be closed
	select {
	case _, ok := <-watcher.Changes():
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestWatcher_EmitsOldAndNewConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	initial := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode"}
		}
	}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	watcher := NewWatcher(path, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Start(ctx)

	// Wait a tick, then update
	time.Sleep(100 * time.Millisecond)
	updated := `{
		"workspace": "/tmp/updated",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {"type": "opencode", "binary": "opencode"}
		}
	}`
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case change := <-watcher.Changes():
		if change.Old == nil {
			t.Fatal("expected non-nil old config")
		}
		if change.New == nil {
			t.Fatal("expected non-nil new config")
		}
		if change.Old.Workspace != "/tmp/test" {
			t.Errorf("expected old workspace /tmp/test, got %q", change.Old.Workspace)
		}
		if change.New.Workspace != "/tmp/updated" {
			t.Errorf("expected new workspace /tmp/updated, got %q", change.New.Workspace)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for config change")
	}
}
