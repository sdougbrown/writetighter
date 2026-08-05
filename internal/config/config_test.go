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

func TestLLMConfigContextValidation(t *testing.T) {
	// Helper to write a user config file at the expected path under dir.
	writeCfg := func(t *testing.T, dir string, data []byte) {
		t.Helper()
		cfgDir := filepath.Join(dir, "writetighter")
		if err := os.MkdirAll(cfgDir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(cfgDir, "config.toml")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("valid with both tokens", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, []byte("[llm]\nprovider='openai-compatible'\nbase_url='http://localhost:4000/v1'\nmodel='gemma4'\ncontext_window_tokens=8192\nmax_output_tokens=4096\n"))
		t.Setenv("XDG_CONFIG_HOME", dir)
		cfg, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("expected valid config to load, got: %v", err)
		}
		if cfg.LLM.ContextWindowTokens != 8192 {
			t.Fatalf("expected context_window_tokens=8192, got %d", cfg.LLM.ContextWindowTokens)
		}
		if cfg.LLM.MaxOutputTokens != 4096 {
			t.Fatalf("expected max_output_tokens=4096, got %d", cfg.LLM.MaxOutputTokens)
		}
	})

	t.Run("rejects context_window_tokens=0", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, []byte("[llm]\nprovider='openai-compatible'\ncontext_window_tokens=0\n"))
		t.Setenv("XDG_CONFIG_HOME", dir)
		_, err := LoadUserConfig()
		if err == nil {
			t.Fatal("expected error for context_window_tokens=0")
		}
		if !strings.Contains(err.Error(), "context_window_tokens must be > 0") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects negative context_window_tokens", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, []byte("[llm]\nprovider='openai-compatible'\ncontext_window_tokens=-100\n"))
		t.Setenv("XDG_CONFIG_HOME", dir)
		_, err := LoadUserConfig()
		if err == nil {
			t.Fatal("expected error for negative context_window_tokens")
		}
		if !strings.Contains(err.Error(), "context_window_tokens must be > 0") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects max_output_tokens >= context_window_tokens", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, []byte("[llm]\nprovider='openai-compatible'\ncontext_window_tokens=4096\nmax_output_tokens=4096\n"))
		t.Setenv("XDG_CONFIG_HOME", dir)
		_, err := LoadUserConfig()
		if err == nil {
			t.Fatal("expected error for max_output_tokens >= context_window_tokens")
		}
		if !strings.Contains(err.Error(), "must be less than") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects max_output_tokens greater than context", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, []byte("[llm]\nprovider='openai-compatible'\ncontext_window_tokens=4096\nmax_output_tokens=8192\n"))
		t.Setenv("XDG_CONFIG_HOME", dir)
		_, err := LoadUserConfig()
		if err == nil {
			t.Fatal("expected error for max_output_tokens > context_window_tokens")
		}
		if !strings.Contains(err.Error(), "must be less than") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("round-trip through write and load", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", root)
		orig := &UserConfig{LLM: LLMConfig{
			Provider:            "openai-compatible",
			BaseURL:             "http://localhost:4000/v1",
			Model:               "gemma4",
			APIKeyEnv:           "WRITETIGHTER_API_KEY",
			ResponseMode:        "json_object",
			ContextWindowTokens: 8192,
			MaxOutputTokens:     4096,
		}}
		path, err := WriteUserConfig(orig)
		if err != nil {
			t.Fatalf("WriteUserConfig failed: %v", err)
		}
		loaded, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("LoadUserConfig after write failed: %v", err)
		}
		if loaded.LLM.ContextWindowTokens != 8192 {
			t.Fatalf("round-trip context_window_tokens: got %d, want 8192", loaded.LLM.ContextWindowTokens)
		}
		if loaded.LLM.MaxOutputTokens != 4096 {
			t.Fatalf("round-trip max_output_tokens: got %d, want 4096", loaded.LLM.MaxOutputTokens)
		}
		if loaded.LLM.ContextWindowModel != "" {
			t.Fatalf("round-trip context_window_model: got %q, want empty", loaded.LLM.ContextWindowModel)
		}
		_ = path
	})

	t.Run("backward compat without new fields", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, []byte("[llm]\nprovider='openai-compatible'\nbase_url='http://localhost:4000/v1'\nmodel='gemma4'\napi_key_env='WRITETIGHTER_API_KEY'\nresponse_mode='json_object'\n"))
		t.Setenv("XDG_CONFIG_HOME", dir)
		cfg, err := LoadUserConfig()
		if err != nil {
			t.Fatalf("expected backward-compat config to load: %v", err)
		}
		if cfg.LLM.ContextWindowTokens != 0 || cfg.LLM.MaxOutputTokens != 0 || cfg.LLM.ContextWindowModel != "" {
			t.Fatalf("new fields should be zero-valued when absent: %+v", cfg.LLM)
		}
	})
}
