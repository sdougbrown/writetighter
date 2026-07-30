package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTermBase(t *testing.T) {
	if err := ValidateTermBase([]TermEntry{{Term: ""}}); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateTermBase([]TermEntry{{Term: "A"}, {Term: "a"}}); err == nil {
		t.Fatal("expected duplicate error")
	}
	if err := ValidateTermBase([]TermEntry{{Term: "A", Override: true}}); err == nil {
		t.Fatal("expected override reason error")
	}
}

func TestLoadAndDiscoverProjectConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".writetighter.toml")
	data := []byte("[profile]\nid='core'\nversion='1'\n[[terms]]\nterm='alpha'\ndefinition='x'\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile.ID != "core" {
		t.Fatal("bad profile id")
	}
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	_, discovered, err := DiscoverProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	if discovered != path {
		t.Fatalf("got %s", discovered)
	}
}

func TestLoadUserConfigXDG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "writetighter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[llm]\nprovider='x'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg, err := LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Provider != "x" {
		t.Fatal("bad provider")
	}
}

func TestProjectConfigRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".writetighter.toml")
	data := []byte("[profile]\nid='core'\nversion='1'\n[[terms]]\nterm='alpha'\ndefinition='x'\n[llm]\napi_key='sk-abc123'\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown [llm] section in project config")
	}
	t.Logf("got expected error: %v", err)
}

func TestUserConfigAcceptsStoredAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "writetighter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[llm]\nprovider='openai-compatible'\napi_key='pat-local'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg, err := LoadUserConfig()
	if err != nil || cfg.LLM.APIKey != "pat-local" {
		t.Fatalf("stored API key was not loaded: %#v, err=%v", cfg, err)
	}
}

func TestUserConfigRejectsTwoAPIKeySources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "writetighter", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte("[llm]\napi_key='pat-local'\napi_key_env='WRITETIGHTER_API_KEY'\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	_, err := LoadUserConfig()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected conflicting API key sources to fail, got %v", err)
	}
}

func TestWriteUserConfigUsesXDGPathAndPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	cfg := &UserConfig{LLM: LLMConfig{
		Provider: "openai-compatible", BaseURL: "http://localhost:4000/v1",
		Model: "gemma4", APIKeyEnv: "WRITETIGHTER_API_KEY", ResponseMode: "json_object",
	}}
	path, err := WriteUserConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "writetighter", "config.toml")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LLM.Model != "gemma4" || loaded.LLM.APIKeyEnv != "WRITETIGHTER_API_KEY" {
		t.Fatalf("unexpected config: %#v", loaded)
	}
}

func TestSanitizedTOMLRedactsAPIKey(t *testing.T) {
	cfg := &UserConfig{LLM: LLMConfig{
		Provider:     "openai-compatible",
		BaseURL:      "http://localhost:4000/v1",
		Model:        "test-model",
		APIKey:       "sk-secret-key-12345",
		Timeout:      "45s",
		ResponseMode: "json_object",
		MaxRequests:  32,
	}}
	tomlStr, err := cfg.SanitizedTOML()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tomlStr, "sk-secret-key-12345") {
		t.Fatalf("API key was not redacted: %s", tomlStr)
	}
	if !strings.Contains(tomlStr, "test-model") {
		t.Fatalf("model name was lost: %s", tomlStr)
	}
	if !strings.Contains(tomlStr, "http://localhost:4000/v1") {
		t.Fatalf("base_url was lost: %s", tomlStr)
	}
}

func TestSanitizedTOMLPreservesAPIKeyEnv(t *testing.T) {
	cfg := &UserConfig{LLM: LLMConfig{
		Provider:     "openai-compatible",
		BaseURL:      "http://localhost:4000/v1",
		Model:        "test-model",
		APIKeyEnv:    "WRITETIGHTER_API_KEY",
		Timeout:      "45s",
		ResponseMode: "json_object",
	}}
	tomlStr, err := cfg.SanitizedTOML()
	if err != nil {
		t.Fatal(err)
	}
	// api_key_env is an env var name, not a secret — should be visible.
	if !strings.Contains(tomlStr, "WRITETIGHTER_API_KEY") {
		t.Fatalf("api_key_env was lost: %s", tomlStr)
	}
}

func TestProjectConfigRejectsApiKeyEnvInProject(t *testing.T) {
	// Project config should reject [llm] sections entirely.
	dir := t.TempDir()
	path := filepath.Join(dir, ".writetighter.toml")
	data := []byte("[profile]\nid='core'\nversion='1'\n[llm]\napi_key_env='MY_KEY'\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProjectConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown [llm] section in project config")
	}
	t.Logf("got expected error: %v", err)
}
