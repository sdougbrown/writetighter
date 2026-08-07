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
	"time"
	"unicode"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/llm"
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

func TestRunWritesSelectedModelWithoutAuthentication(t *testing.T) {
	server := setupServer(t, "", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	// No context window or output tokens prompts in the new wizard.
	input := strings.NewReader(server.URL + "/v1\n1\n2\n")
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
	// Context window and output tokens are no longer stored in config.
	if cfg.LLM.CodeModel != "" {
		t.Fatalf("code_model should be empty by default, got %q", cfg.LLM.CodeModel)
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
	// Select model 1 (gemma4). No context/output prompts.
	input := strings.NewReader(server.URL + "/v1\n2\n1\n")
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
	if cfg.LLM.Model != "gemma4" {
		t.Fatalf("model = %q, want gemma4", cfg.LLM.Model)
	}
}

func TestRunCanUseEnvironmentVariable(t *testing.T) {
	server := setupServer(t, "pat-env", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("MY_MODEL_PAT", "pat-env")
	// Select model 1 (gemma4). No context/output prompts.
	input := strings.NewReader(server.URL + "/v1\n3\nMY_MODEL_PAT\n1\n")
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
	if cfg.LLM.Model != "gemma4" {
		t.Fatalf("model = %q, want gemma4", cfg.LLM.Model)
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
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[llm\nbroken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	// Replace invalid config, select model 1 (gemma4). No context/output prompts.
	input := strings.NewReader("yes\n" + server.URL + "/v1\n1\n1\n")
	if _, err := Run(context.Background(), Options{In: input, Out: io.Discard, HTTPClient: server.Client()}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil || cfg.LLM.Model != "gemma4" {
		t.Fatalf("invalid config was not replaced: %#v, err=%v", cfg, err)
	}
}

func TestRunFallsBackToPromptJSON(t *testing.T) {
	server := setupServer(t, "", true)
	defer server.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// No context/output prompts.
	input := strings.NewReader(server.URL + "/v1\n1\n1\n")
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
	// No context/output prompts.
	input := strings.NewReader(server.URL + "/v1\n1\n1\n")
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
}

func TestRunPreservesExistingCodeModel(t *testing.T) {
	server := setupServer(t, "", false)
	defer server.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	// Write an initial config with code_model set.
	initial := &config.UserConfig{LLM: config.LLMConfig{
		Provider:     "openai-compatible",
		BaseURL:      server.URL + "/v1",
		Model:        "old-model",
		ResponseMode: "json_object",
		CodeModel:    "qwen-coder",
	}}
	if _, err := config.WriteUserConfig(initial); err != nil {
		t.Fatal(err)
	}
	// Re-run wizard, select model 2 (qwen). No context/output prompts.
	input := strings.NewReader(server.URL + "/v1\n1\n2\n")
	if _, err := Run(context.Background(), Options{In: input, Out: io.Discard, HTTPClient: server.Client()}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.CodeModel != "qwen-coder" {
		t.Fatalf("code_model = %q, want qwen-coder (should be preserved across wizard runs)", cfg.LLM.CodeModel)
	}
}

func TestListModelsViaLLMPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "gemma4", "context_length": 4096}, {"id": "bad\x1b[2J"}}})
	}))
	defer server.Close()
	models, err := llm.ListModels(server.URL, "", 30*time.Second)
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

func TestDoSanitizesControlCharactersInNon2xxBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("\x1b[2Jbad response\n"))
	}))
	defer server.Close()

	client := server.Client()
	ctx := context.Background()

	_, err := doJSON(ctx, client, http.MethodGet, server.URL+"/", "", nil)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}

	errStr := err.Error()
	// Should contain status and message content.
	if !strings.Contains(errStr, "HTTP 418") {
		t.Fatalf("error should contain HTTP status: %s", errStr)
	}
	if !strings.Contains(errStr, "bad response") {
		t.Fatalf("error should contain body message: %s", errStr)
	}
	// Should NOT contain any control characters from the response body.
	for i, r := range errStr {
		if unicode.IsControl(r) {
			t.Fatalf("error contains control character at byte index %d: %q", i, string(r))
		}
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
