// Package config handles loading and validating the voilot configuration file.
// The config file lives at ~/.config/voilot/config.json by default and can be
// overridden via the --config CLI flag.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultPath returns the default config file path (~/.config/voilot/config.json).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "voilot", "config.json")
}

// ProviderConfig describes a single agent provider.
type ProviderConfig struct {
	Type   string `json:"type"`             // provider type: "opencode", "claude-code", "pi"
	Binary string `json:"binary,omitempty"` // path or name of the binary (defaults to type name)
}

// Config is the top-level voilot configuration.
type Config struct {
	Workspace       string                    `json:"workspace"`
	DefaultProvider string                    `json:"defaultProvider"`
	Providers       map[string]ProviderConfig `json:"providers"`
	MaxInstances    int                       `json:"maxInstances,omitempty"`
	IdleTimeout     string                    `json:"idleTimeout,omitempty"` // e.g. "10m"
	TTSUrl          string                    `json:"ttsUrl,omitempty"`
	STTUrl          string                    `json:"sttUrl,omitempty"`
}

// IdleTimeoutDuration parses the IdleTimeout string into a time.Duration.
// Returns the default (10m) if empty or unparseable.
func (c *Config) IdleTimeoutDuration() time.Duration {
	if c.IdleTimeout == "" {
		return 10 * time.Minute
	}
	d, err := time.ParseDuration(c.IdleTimeout)
	if err != nil {
		return 10 * time.Minute
	}
	return d
}

// Load reads and validates a config file at the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s\n\nCreate one with the following structure:\n%s", path, SampleConfig())
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	cfg.applyDefaults()
	return &cfg, nil
}

// Validate checks that the config has all required fields and valid values.
func (c *Config) Validate() error {
	if c.Workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if len(c.Providers) == 0 {
		return fmt.Errorf("at least one provider must be configured")
	}
	if c.DefaultProvider == "" {
		return fmt.Errorf("defaultProvider is required")
	}
	if _, ok := c.Providers[c.DefaultProvider]; !ok {
		return fmt.Errorf("defaultProvider %q not found in providers", c.DefaultProvider)
	}

	supportedTypes := map[string]bool{"opencode": true}
	for name, p := range c.Providers {
		if p.Type == "" {
			return fmt.Errorf("provider %q: type is required", name)
		}
		if !supportedTypes[p.Type] {
			return fmt.Errorf("provider %q: unsupported type %q (supported: opencode)", name, p.Type)
		}
	}

	if c.MaxInstances < 0 {
		return fmt.Errorf("maxInstances must be >= 0")
	}

	if c.IdleTimeout != "" {
		if _, err := time.ParseDuration(c.IdleTimeout); err != nil {
			return fmt.Errorf("idleTimeout: %w", err)
		}
	}

	return nil
}

func (c *Config) applyDefaults() {
	if c.MaxInstances == 0 {
		c.MaxInstances = 5
	}
	for name, p := range c.Providers {
		if p.Binary == "" {
			p.Binary = p.Type
			c.Providers[name] = p
		}
	}
}

// SampleConfig returns a sample config JSON string for documentation.
func SampleConfig() string {
	sample := Config{
		Workspace:       "/path/to/workspace",
		DefaultProvider: "opencode",
		Providers: map[string]ProviderConfig{
			"opencode": {
				Type:   "opencode",
				Binary: "opencode",
			},
		},
		MaxInstances: 5,
		IdleTimeout:  "10m",
		TTSUrl:       "http://localhost:8880",
		STTUrl:       "http://localhost:5003",
	}
	data, _ := json.MarshalIndent(sample, "", "  ")
	return string(data)
}
