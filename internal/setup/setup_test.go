package setup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/config"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := map[string]string{
		"4000":                     "http://localhost:4000/v1",
		"sparky:4000":              "http://sparky:4000/v1",
		"https://api.example.test": "https://api.example.test/v1",
		"https://api.example.test/openai/v1/models": "https://api.example.test/openai/v1",
	}
	for input, want := range tests {
		got, err := NormalizeBaseURL(input)
		if err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestListModelsDropsControlCharacterIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "gemma4", "context_length": 4096}, {"id": "bad\x1b[2J"}}})
	}))
	defer server.Close()
	models, err := ListModels(context.Background(), server.Client(), server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gemma4" {
		t.Fatalf("models = %#v", models)
	}
	if models[0].SuggestedContextWindow() != 4096 {
		t.Fatalf("SuggestedContextWindow = %d, want 4096", models[0].SuggestedContextWindow())
	}
}

func TestRunWritesSelectedModelWithoutAuthentication(t *testing.T) {
	server := setupServer(t, "", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	// Accept default context (8192 for qwen) and default max output tokens (2048).
	input := strings.NewReader(server.URL + "/v1\n1\n2\n\n\n")
	var output strings.Builder
	result, err := Run(context.Background(), Options{In: input, Out: &output, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "qwen" || result.ResponseMode != "json_schema" {
		t.Fatalf("unexpected result: %#v", result)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Model != "qwen" || cfg.LLM.MaxRequests != 32 || cfg.LLM.APIKey != "" || cfg.LLM.APIKeyEnv != "" || cfg.LLM.ResponseMode != "json_schema" {
		t.Fatalf("unexpected config: %#v", cfg.LLM)
	}
	if cfg.LLM.ContextWindowTokens != 8192 {
		t.Fatalf("context_window_tokens = %d, want 8192", cfg.LLM.ContextWindowTokens)
	}
	if cfg.LLM.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", cfg.LLM.MaxOutputTokens)
	}
	if cfg.LLM.ContextWindowModel != "qwen" {
		t.Fatalf("context_window_model = %q, want qwen", cfg.LLM.ContextWindowModel)
	}
	info, err := os.Stat(filepath.Join(root, "writetighter", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %v", info.Mode().Perm())
	}
}

func TestRunCanStoreAPIKeyInPrivateConfig(t *testing.T) {
	server := setupServer(t, "pat-local", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	// Select model 1 (gemma4), accept default context (4096) and output (2048).
	input := strings.NewReader(server.URL + "/v1\n2\n1\n\n\n")
	result, err := Run(context.Background(), Options{
		In: input, Out: io.Discard, HTTPClient: server.Client(),
		ReadSecret: func(string) (string, error) { return "pat-local", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NeedsEnvironment || result.APIKeyEnv != "" {
		t.Fatalf("unexpected environment requirement: %#v", result)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "pat-local" || cfg.LLM.APIKeyEnv != "" {
		t.Fatalf("stored key config = %#v", cfg.LLM)
	}
	if cfg.LLM.ContextWindowTokens != 4096 {
		t.Fatalf("context_window_tokens = %d, want 4096", cfg.LLM.ContextWindowTokens)
	}
	if cfg.LLM.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", cfg.LLM.MaxOutputTokens)
	}
	if cfg.LLM.ContextWindowModel != "gemma4" {
		t.Fatalf("context_window_model = %q, want gemma4", cfg.LLM.ContextWindowModel)
	}
}

func TestRunCanUseEnvironmentVariable(t *testing.T) {
	server := setupServer(t, "pat-env", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("MY_MODEL_PAT", "pat-env")
	// Select model 1 (gemma4), accept default context (4096) and output (2048).
	input := strings.NewReader(server.URL + "/v1\n3\nMY_MODEL_PAT\n1\n\n\n")
	result, err := Run(context.Background(), Options{In: input, Out: io.Discard, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if result.APIKeyEnv != "MY_MODEL_PAT" || result.NeedsEnvironment {
		t.Fatalf("unexpected result: %#v", result)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "" || cfg.LLM.APIKeyEnv != "MY_MODEL_PAT" {
		t.Fatalf("environment key config = %#v", cfg.LLM)
	}
	if cfg.LLM.ContextWindowTokens != 4096 {
		t.Fatalf("context_window_tokens = %d, want 4096", cfg.LLM.ContextWindowTokens)
	}
	if cfg.LLM.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", cfg.LLM.MaxOutputTokens)
	}
	if cfg.LLM.ContextWindowModel != "gemma4" {
		t.Fatalf("context_window_model = %q, want gemma4", cfg.LLM.ContextWindowModel)
	}
}

func TestRunCanReplaceInvalidExistingConfig(t *testing.T) {
	server := setupServer(t, "", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	dir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[llm]\nunknown='value'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Replace invalid config, select model 1 (gemma4), accept context/output defaults.
	input := strings.NewReader("yes\n" + server.URL + "/v1\n1\n1\n\n\n")
	if _, err := Run(context.Background(), Options{In: input, Out: io.Discard, HTTPClient: server.Client()}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil || cfg.LLM.Model != "gemma4" {
		t.Fatalf("invalid config was not replaced: %#v, err=%v", cfg, err)
	}
	if cfg.LLM.ContextWindowTokens != 4096 {
		t.Fatalf("context_window_tokens = %d, want 4096", cfg.LLM.ContextWindowTokens)
	}
	if cfg.LLM.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", cfg.LLM.MaxOutputTokens)
	}
}

func TestRunFallsBackToPromptJSON(t *testing.T) {
	server := setupServer(t, "", true)
	defer server.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Accept default context (4096 for gemma4) and output (2048).
	input := strings.NewReader(server.URL + "/v1\n1\n1\n\n\n")
	var output strings.Builder
	result, err := Run(context.Background(), Options{In: input, Out: &output, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseMode != "prompt_json" {
		t.Fatalf("response mode = %q", result.ResponseMode)
	}
	// Verify the full 3-step cascade was exercised, not just the final mode.
	for _, expected := range []string{"json_schema was not accepted", "json_object was not accepted", "trying prompt-only JSON"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing fallback log %q in output: %s", expected, output.String())
		}
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.ResponseMode != "prompt_json" {
		t.Fatalf("persisted response mode = %q, want prompt_json", cfg.LLM.ResponseMode)
	}
	if cfg.LLM.ContextWindowTokens != 4096 {
		t.Fatalf("context_window_tokens = %d, want 4096", cfg.LLM.ContextWindowTokens)
	}
	if cfg.LLM.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", cfg.LLM.MaxOutputTokens)
	}
}

func TestRunContextCapacityPromptAcceptsCustomValues(t *testing.T) {
	server := setupServer(t, "", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	// Select model 1 (gemma4, suggests 4096), enter 8192 for context, 1024 for output.
	input := strings.NewReader(server.URL + "/v1\n1\n1\n8192\n1024\n")
	result, err := Run(context.Background(), Options{In: input, Out: io.Discard, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "gemma4" {
		t.Fatalf("model = %q, want gemma4", result.Model)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.ContextWindowTokens != 8192 {
		t.Fatalf("context_window_tokens = %d, want 8192", cfg.LLM.ContextWindowTokens)
	}
	if cfg.LLM.MaxOutputTokens != 1024 {
		t.Fatalf("max_output_tokens = %d, want 1024", cfg.LLM.MaxOutputTokens)
	}
	if cfg.LLM.ContextWindowModel != "gemma4" {
		t.Fatalf("context_window_model = %q, want gemma4", cfg.LLM.ContextWindowModel)
	}
}

func TestRunFallsBackFromJSONSchemaToJSONObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "qwen"}}})
		case "/v1/chat/completions":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if rf, ok := request["response_format"].(map[string]any); ok && rf["type"] == "json_schema" {
				http.Error(w, "json_schema unsupported", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"ok":true}`}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Model qwen has no context metadata from this server; accept defaults (0 context, 2048 output).
	input := strings.NewReader(server.URL + "/v1\n1\n1\n\n\n")
	result, err := Run(context.Background(), Options{In: input, Out: io.Discard, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseMode != "json_object" {
		t.Fatalf("response mode = %q, want json_object", result.ResponseMode)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.ResponseMode != "json_object" {
		t.Fatalf("persisted response mode = %q, want json_object", cfg.LLM.ResponseMode)
	}
	// No context metadata; context should be 0, output should be default 2048.
	if cfg.LLM.ContextWindowTokens != 0 {
		t.Fatalf("context_window_tokens = %d, want 0", cfg.LLM.ContextWindowTokens)
	}
	if cfg.LLM.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", cfg.LLM.MaxOutputTokens)
	}
	if cfg.LLM.ContextWindowModel != "qwen" {
		t.Fatalf("context_window_model = %q, want qwen", cfg.LLM.ContextWindowModel)
	}
}

func setupServer(t *testing.T, requiredKey string, rejectResponseFormat bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requiredKey != "" && r.Header.Get("Authorization") != "Bearer "+requiredKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": "qwen", "context_length": 8192},
				{"id": "gemma4", "context_length": 4096},
			}})
		case "/v1/chat/completions":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if rejectResponseFormat && request["response_format"] != nil {
				http.Error(w, "response_format unsupported", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"ok":true}`}}}})
		default:
			http.NotFound(w, r)
		}
	}))
}
