package config

import (
	"os"
	"strings"
	"testing"
)

func TestValidate_EnvKeys(t *testing.T) {
	base := Config{
		Workspace:       "/tmp/test",
		DefaultProvider: "opencode",
		Providers: map[string]ProviderConfig{
			"opencode": {Type: "opencode", Binary: "opencode"},
		},
	}

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{"valid simple key", "MY_VAR", "some-value", false},
		{"valid underscore start", "_VAR", "value", false},
		{"valid mixed case", "myVar123", "value", false},
		{"valid single char", "X", "value", false},
		{"invalid starts with digit", "1BAD", "value", true},
		{"invalid contains dash", "MY-VAR", "value", true},
		{"invalid contains space", "MY VAR", "value", true},
		{"invalid contains dot", "MY.VAR", "value", true},
		{"invalid empty key", "", "value", true},
		{"empty value", "GOOD_KEY", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.Providers = map[string]ProviderConfig{
				"opencode": {
					Type:   "opencode",
					Binary: "opencode",
					Env:    map[string]string{tt.key: tt.value},
				},
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_EnvEmpty(t *testing.T) {
	cfg := Config{
		Workspace:       "/tmp/test",
		DefaultProvider: "opencode",
		Providers: map[string]ProviderConfig{
			"opencode": {
				Type:   "opencode",
				Binary: "opencode",
				Env:    map[string]string{},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with empty env map should not error, got: %v", err)
	}
}

func TestValidate_NoEnv(t *testing.T) {
	cfg := Config{
		Workspace:       "/tmp/test",
		DefaultProvider: "opencode",
		Providers: map[string]ProviderConfig{
			"opencode": {
				Type:   "opencode",
				Binary: "opencode",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with nil env map should not error, got: %v", err)
	}
}

func TestLoad_InvalidEnvKey(t *testing.T) {
	// Write a temp config file with an invalid env key
	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"123INVALID": "some-value"
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("Load() should return error for invalid env key")
	}
}

func TestLoad_EmptyEnvValue(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"GOOD_KEY": ""
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("Load() should return error for empty env value")
	}
}

func TestLoad_ValidEnv(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"AWS_BEARER_TOKEN_BEDROCK": "my-token-value",
					"ANOTHER_VAR": "another-value"
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	env := cfg.Providers["opencode"].Env
	if env["AWS_BEARER_TOKEN_BEDROCK"] != "my-token-value" {
		t.Errorf("expected env AWS_BEARER_TOKEN_BEDROCK = 'my-token-value', got %q", env["AWS_BEARER_TOKEN_BEDROCK"])
	}
	if env["ANOTHER_VAR"] != "another-value" {
		t.Errorf("expected env ANOTHER_VAR = 'another-value', got %q", env["ANOTHER_VAR"])
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// --- ${VAR} reference tests ---

func TestValidate_EnvRefFormat(t *testing.T) {
	base := Config{
		Workspace:       "/tmp/test",
		DefaultProvider: "opencode",
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid reference", "${MY_VAR}", false},
		{"valid reference underscore start", "${_VAR}", false},
		{"valid reference mixed case", "${myVar123}", false},
		{"invalid ref starts with digit", "${123BAD}", true},
		{"invalid ref no closing brace", "${VAR", true},
		{"invalid ref mixed content prefix", "prefix${VAR}", true},
		{"invalid ref mixed content suffix", "${VAR}suffix", true},
		{"invalid ref concatenated", "${A}${B}", true},
		{"invalid ref empty name", "${}", true},
		{"literal with dollar no brace", "$VAR", false}, // not a reference, just a literal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.Providers = map[string]ProviderConfig{
				"opencode": {
					Type:   "opencode",
					Binary: "opencode",
					Env:    map[string]string{"TEST_KEY": tt.value},
				},
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveEnvRefs_ExpandsFromProcessEnv(t *testing.T) {
	// Set a test env var
	t.Setenv("VOILOT_TEST_TOKEN", "expanded-value-123")

	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"MY_TOKEN": "${VOILOT_TEST_TOKEN}"
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	got := cfg.Providers["opencode"].Env["MY_TOKEN"]
	if got != "expanded-value-123" {
		t.Errorf("expected expanded value 'expanded-value-123', got %q", got)
	}
}

func TestResolveEnvRefs_UnsetVarFails(t *testing.T) {
	// Ensure the var is NOT set
	os.Unsetenv("VOILOT_NONEXISTENT_VAR_XYZ")

	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"MY_TOKEN": "${VOILOT_NONEXISTENT_VAR_XYZ}"
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() should fail when ${VAR} references unset variable")
	}
	// Verify error message names the variable
	if !strings.Contains(err.Error(), "VOILOT_NONEXISTENT_VAR_XYZ") {
		t.Errorf("error should name the unset variable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not set in the process environment") {
		t.Errorf("error should say 'not set in the process environment', got: %v", err)
	}
}

func TestResolveEnvRefs_EmptyVarFails(t *testing.T) {
	// Set the var to empty string
	t.Setenv("VOILOT_EMPTY_VAR", "")

	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"MY_TOKEN": "${VOILOT_EMPTY_VAR}"
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() should fail when ${VAR} references empty variable")
	}
	if !strings.Contains(err.Error(), "VOILOT_EMPTY_VAR") {
		t.Errorf("error should name the variable, got: %v", err)
	}
}

func TestResolveEnvRefs_LiteralsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"LITERAL_VAL": "my-literal-token-value"
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	got := cfg.Providers["opencode"].Env["LITERAL_VAL"]
	if got != "my-literal-token-value" {
		t.Errorf("expected literal value unchanged, got %q", got)
	}
}

func TestResolveEnvRefs_MixedLiteralAndRef(t *testing.T) {
	t.Setenv("VOILOT_TEST_SECRET", "secret-789")

	dir := t.TempDir()
	path := dir + "/config.json"
	data := `{
		"workspace": "/tmp/test",
		"defaultProvider": "opencode",
		"providers": {
			"opencode": {
				"type": "opencode",
				"binary": "opencode",
				"env": {
					"FROM_ENV": "${VOILOT_TEST_SECRET}",
					"LITERAL": "plain-value"
				}
			}
		}
	}`
	if err := writeTestFile(path, data); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	env := cfg.Providers["opencode"].Env
	if env["FROM_ENV"] != "secret-789" {
		t.Errorf("expected FROM_ENV = 'secret-789', got %q", env["FROM_ENV"])
	}
	if env["LITERAL"] != "plain-value" {
		t.Errorf("expected LITERAL = 'plain-value', got %q", env["LITERAL"])
	}
}

func TestValidate_EnvRefErrorMessageFormat(t *testing.T) {
	// Test that literal empty and ref unset produce structurally consistent messages
	cfg := Config{
		Workspace:       "/tmp/test",
		DefaultProvider: "opencode",
		Providers: map[string]ProviderConfig{
			"opencode": {
				Type:   "opencode",
				Binary: "opencode",
				Env:    map[string]string{"MY_KEY": ""},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	// Both error formats should include: provider name, env key name
	if !strings.Contains(err.Error(), `"opencode"`) {
		t.Errorf("error should contain provider name, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"MY_KEY"`) {
		t.Errorf("error should contain key name, got: %v", err)
	}
}
