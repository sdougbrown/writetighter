package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/app"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/profile"
)

// captureStdout runs fn with os.Stdout replaced by a pipe and returns the captured text.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	done := make(chan struct{})
	var out bytes.Buffer
	go func() {
		io.Copy(&out, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	return out.String()
}

// captureStdoutStderr captures both streams and returns (stdout, stderr).
func captureStdoutStderr(t *testing.T, fn func()) (string, string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	doneOut, doneErr := make(chan struct{}), make(chan struct{})
	var outBuf, errBuf bytes.Buffer
	go func() {
		io.Copy(&outBuf, rOut)
		close(doneOut)
	}()
	go func() {
		io.Copy(&errBuf, rErr)
		close(doneErr)
	}()
	fn()
	_ = wOut.Close()
	_ = wErr.Close()
	<-doneOut
	<-doneErr
	return outBuf.String(), errBuf.String()
}

func TestRunVersion(t *testing.T) {
	// --json: assert exit code and decode the payload
	t.Run("json", func(t *testing.T) {
		var payload struct {
			Version          string `json:"version"`
			Commit           string `json:"commit"`
			EmbeddedProfiles []struct {
				ID      string `json:"id"`
				Version string `json:"version"`
			} `json:"embedded_profiles"`
		}
		out := captureStdout(t, func() {
			if got := run([]string{"version", "--json"}); got != 0 {
				t.Fatalf("exit %d", got)
			}
		})
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
		if payload.Version == "" {
			t.Fatal("missing version in JSON payload")
		}
		if payload.Commit == "" {
			t.Fatal("missing commit in JSON payload")
		}
	})

	// bare version: assert stdout contains the version string
	t.Run("human-readable", func(t *testing.T) {
		out := captureStdout(t, func() {
			if got := run([]string{"version"}); got != 0 {
				t.Fatalf("exit %d", got)
			}
		})
		if !strings.Contains(out, "writetighter "+app.Version) {
			t.Fatalf("expected 'writetighter %s' in output, got: %s", app.Version, out)
		}
		// default commit is "unknown" — must not appear in output
		if strings.Contains(out, "(commit") {
			t.Fatalf("commit should not appear when unknown, got: %s", out)
		}
		// an embedded profile exists in the test binary — line must be present
		if !strings.Contains(out, "embedded profile:") {
			t.Fatalf("expected 'embedded profile:' in output, got: %s", out)
		}
	})

	// commit present: seed a real commit and verify it appears
	t.Run("commit-present", func(t *testing.T) {
		oldCommit := app.Commit
		app.Commit = "abc1234"
		defer func() { app.Commit = oldCommit }()
		out := captureStdout(t, func() {
			if got := run([]string{"version"}); got != 0 {
				t.Fatalf("exit %d", got)
			}
		})
		if !strings.Contains(out, "(commit abc1234)") {
			t.Fatalf("expected '(commit abc1234)' in output, got: %s", out)
		}
	})

	// commit unknown: explicitly verify it is suppressed
	t.Run("commit-unknown", func(t *testing.T) {
		oldCommit := app.Commit
		app.Commit = "unknown"
		defer func() { app.Commit = oldCommit }()
		out := captureStdout(t, func() {
			if got := run([]string{"version"}); got != 0 {
				t.Fatalf("exit %d", got)
			}
		})
		if strings.Contains(out, "(commit") {
			t.Fatalf("commit should not appear when 'unknown', got: %s", out)
		}
	})

	// no embedded profile: override the loader to return nil
	t.Run("no-embedded-profile", func(t *testing.T) {
		oldLoader := loadEmbedded
		loadEmbedded = func() (*profile.Resolution, error) { return nil, nil }
		defer func() { loadEmbedded = oldLoader }()
		out := captureStdout(t, func() {
			if got := run([]string{"version"}); got != 0 {
				t.Fatalf("exit %d", got)
			}
		})
		if strings.Contains(out, "embedded profile:") {
			t.Fatalf("embedded profile line should be absent, got: %s", out)
		}
		if !strings.Contains(out, "writetighter "+app.Version) {
			t.Fatalf("expected 'writetighter %s' in output, got: %s", app.Version, out)
		}

		// --json with no embedded profile: embedded_profiles must be empty
		jsonOut := captureStdout(t, func() {
			if got := run([]string{"version", "--json"}); got != 0 {
				t.Fatalf("exit %d", got)
			}
		})
		var payload struct {
			EmbeddedProfiles []struct {
				ID string `json:"id"`
			} `json:"embedded_profiles"`
		}
		if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, jsonOut)
		}
		if len(payload.EmbeddedProfiles) != 0 {
			t.Fatalf("expected empty embedded_profiles, got %d items", len(payload.EmbeddedProfiles))
		}
	})
}

func TestRunExplain(t *testing.T) {
	t.Run("flags before rule", func(t *testing.T) {
		old := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		os.Stdout = w
		defer func() { os.Stdout = old }()

		if got := run([]string{"explain", "--format", "json", "CORE.SENTENCE_LENGTH"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
		_ = w.Close()
		var out bytes.Buffer
		if _, err := io.Copy(&out, r); err != nil {
			t.Fatalf("copy stdout: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		if payload["id"] != "CORE.SENTENCE_LENGTH" {
			t.Fatalf("unexpected id: %v", payload["id"])
		}
	})
}

func TestRunPrompt(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	if got := run([]string{"prompt", "--kind", "code-comment", "--format", "json"}); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
	_ = w.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "code-comment" {
		t.Fatalf("unexpected prompt payload: %v", payload)
	}
}

func TestStrictFlagAndEnumValidation(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--wat"}, {"lint", "--stdin", "--kind", "blog"},
		{"lint", "--stdin", "--format", "yaml"}, {"explain", "--wat", "CORE.SENTENCE_LENGTH"},
		{"prompt", "--kind", "message"}, {"prompt", "--format", "yaml"}, {"prompt", "extra"},
		{"profile", "list", "--wat"},
	} {
		if got := run(args); got != 2 {
			t.Fatalf("%v: got %d", args, got)
		}
	}
}

func TestLintCommand(t *testing.T) {
	if got := run([]string{"lint"}); got != 2 {
		t.Fatalf("expected exit 2 for missing input, got %d", got)
	}
	if got := run([]string{"lint", "--stdin", "foo"}); got != 2 {
		t.Fatalf("expected exit 2 for stdin+path, got %d", got)
	}
	if got := run([]string{"lint", "--stdin", "--llm"}); got != 2 {
		t.Fatalf("expected exit 2 because lint has no LLM flags, got %d", got)
	}
}

func TestLintAcceptsDirectText(t *testing.T) {
	if got := run([]string{"lint", "--text", "Short direct text.", "--format", "json"}); got != 0 {
		t.Fatalf("expected direct text to lint successfully, got %d", got)
	}
	if got := run([]string{"lint", "--text", "text", "file.md"}); got != 2 {
		t.Fatalf("expected --text and path conflict, got %d", got)
	}
	if got := run([]string{"lint", "--text", "text", "--stdin"}); got != 2 {
		t.Fatalf("expected --text and stdin conflict, got %d", got)
	}
}

func TestLintAcceptedFlags(t *testing.T) {
	// lint accepts paths before flags.
	if got := run([]string{"lint", "missing.txt"}); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestConfigRejectsArguments(t *testing.T) {
	if got := run([]string{"config", "extra"}); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
}

func TestReviseCommand(t *testing.T) {
	if got := run([]string{"revise"}); got != 2 {
		t.Fatalf("expected exit 2 for missing input, got %d", got)
	}
	if got := run([]string{"revise", "--stdin", "foo"}); got != 2 {
		t.Fatalf("expected exit 2 for stdin+path, got %d", got)
	}
}

func TestReviseAcceptsDirectTextFlag(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	if got := run([]string{"revise", "--text", "Short direct text."}); got != 2 {
		t.Fatalf("expected missing model config exit 2 after accepting --text, got %d", got)
	}
	if got := run([]string{"revise", "--text", "text", "file.md"}); got != 2 {
		t.Fatalf("expected --text and path conflict, got %d", got)
	}
}

func TestReviseNoLLMConfigFails(t *testing.T) {
	// Without any config file, revise should fail with a clear message about
	// missing LLM configuration.
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	if got := run([]string{"revise", "missing.txt"}); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
}

func TestCheckCommandDoesNotExist(t *testing.T) {
	if got := run([]string{"check", "--stdin"}); got != 2 {
		t.Fatalf("expected removed check command to return exit 2, got %d", got)
	}
}

// --- Help and self-discovery tests ---

func TestNoArgsShowsHelp(t *testing.T) {
	out := captureStdout(t, func() {
		if got := run(nil); got != 0 {
			t.Fatalf("expected exit 0 for no args, got %d", got)
		}
	})
	if !strings.Contains(out, "USAGE") {
		t.Fatalf("expected help text on stdout, got: %s", out)
	}
	if !strings.Contains(out, "COMMANDS") {
		t.Fatalf("expected command list in help, got: %s", out)
	}
}

func TestTopLevelHelpFlag(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		out := captureStdout(t, func() {
			if got := run(args); got != 0 {
				t.Fatalf("%v: expected exit 0, got %d", args, got)
			}
		})
		if !strings.Contains(out, "COMMANDS") {
			t.Fatalf("%v: expected help text, got: %s", args, out)
		}
	}
}

func TestHelpCommand(t *testing.T) {
	out := captureStdout(t, func() {
		if got := run([]string{"help"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
	})
	if !strings.Contains(out, "COMMANDS") {
		t.Fatalf("expected main help, got: %s", out)
	}
}

func TestHelpCommandForSubcommand(t *testing.T) {
	out := captureStdout(t, func() {
		if got := run([]string{"help", "lint"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
	})
	if !strings.Contains(out, "writetighter lint") {
		t.Fatalf("expected lint help, got: %s", out)
	}
}

func TestSubcommandHelpFlags(t *testing.T) {
	for _, tc := range []struct {
		args     string
		contains string
	}{
		{"lint --help", "--fail-on"},
		{"revise --help", "CONFIGURATION"},
		{"prompt --help", "revision guidance"},
		{"config --help", "--wizard"},
		{"explain --help", "rule-id"},
		{"profile --help", "SUBCOMMANDS"},
		{"version --help", "--json"},
	} {
		t.Run(tc.args, func(t *testing.T) {
			out := captureStdout(t, func() {
				if got := run(strings.Fields(tc.args)); got != 0 {
					t.Fatalf("expected exit 0, got %d", got)
				}
			})
			if !strings.Contains(out, tc.contains) {
				t.Fatalf("expected %q in help output, got: %s", tc.contains, out)
			}
		})
	}
}

func TestUnknownCommandShowsHelp(t *testing.T) {
	out := captureStdout(t, func() {
		if got := run([]string{"bogus"}); got != 2 {
			t.Fatalf("expected exit 2 for unknown command, got %d", got)
		}
	})
	if !strings.Contains(out, "COMMANDS") {
		t.Fatalf("expected help text after unknown command, got: %s", out)
	}
}

func TestConfigShowsSanitizedWhenConfigured(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	configDir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	data := []byte("[llm]\nprovider='openai-compatible'\nbase_url='http://localhost:4000/v1'\nmodel='test-model'\napi_key='sk-secret-key-12345'\ntimeout='45s'\nresponse_mode='json_object'\nmax_requests=32\n")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := captureStdoutStderr(t, func() {
		if got := run([]string{"config"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
	})
	// Config should be shown on stdout.
	if !strings.Contains(stdout, "test-model") {
		t.Fatalf("expected model name in stdout, got: %s", stdout)
	}
	// API key must be redacted everywhere.
	if strings.Contains(stdout, "sk-secret-key-12345") {
		t.Fatalf("API key was not redacted in stdout: %s", stdout)
	}
	if strings.Contains(stderr, "sk-secret-key-12345") {
		t.Fatalf("API key leaked to stderr: %s", stderr)
	}
	// Redaction note on stderr.
	if !strings.Contains(stderr, "redacted") {
		t.Fatalf("expected redaction note on stderr, got: %s", stderr)
	}
	// Should hint at --wizard on stderr.
	if !strings.Contains(stderr, "--wizard") {
		t.Fatalf("expected --wizard hint on stderr, got: %s", stderr)
	}
}

func TestConfigShowsSanitizedWithAPIKeyEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	configDir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	data := []byte("[llm]\nprovider='openai-compatible'\nbase_url='http://localhost:4000/v1'\nmodel='test-model'\napi_key_env='WRITETIGHTER_API_KEY'\ntimeout='45s'\nresponse_mode='json_object'\n")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := captureStdoutStderr(t, func() {
		if got := run([]string{"config"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
	})
	// api_key_env is an env var name, not a secret, so it should be visible.
	if !strings.Contains(stdout, "WRITETIGHTER_API_KEY") {
		t.Fatalf("expected api_key_env in stdout, got: %s", stdout)
	}
	// Should hint at --wizard on stderr.
	if !strings.Contains(stderr, "--wizard") {
		t.Fatalf("expected --wizard hint on stderr, got: %s", stderr)
	}
}

func TestConfigWizardFlagAccepted(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	configDir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	data := []byte("[llm]\nprovider='openai-compatible'\nbase_url='http://localhost:4000/v1'\nmodel='test-model'\napi_key='sk-secret'\n")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// --wizard should not show sanitized config; it should enter the wizard.
	// Since stdin is not a terminal in tests, the wizard will fail, returning 2.
	stdout, _ := captureStdoutStderr(t, func() {
		if got := run([]string{"config", "--wizard"}); got != 2 {
			t.Fatalf("expected exit 2 (wizard fails without terminal), got %d", got)
		}
	})
	// Should not have printed the sanitized config.
	if strings.Contains(stdout, "test-model") {
		t.Fatalf("--wizard should not print sanitized config, got: %s", stdout)
	}
}

func TestConfigShortWizardFlag(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	configDir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	data := []byte("[llm]\nprovider='openai-compatible'\nbase_url='http://localhost:4000/v1'\nmodel='test-model'\napi_key='sk-secret'\n")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// -w is the short form of --wizard.
	stdout, _ := captureStdoutStderr(t, func() {
		if got := run([]string{"config", "-w"}); got != 2 {
			t.Fatalf("expected exit 2 (wizard fails without terminal), got %d", got)
		}
	})
	if strings.Contains(stdout, "test-model") {
		t.Fatalf("-w should not print sanitized config, got: %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Targeted config flag tests
// ---------------------------------------------------------------------------

// configTestServer creates an HTTP server that responds to /v1/models and
// /v1/chat/completions (and the same paths without /v1) for targeted config tests.
func configTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return configTestServerWithChatHandler(t, nil)
}

// configTestServerWithChatHandler creates an HTTP server that responds to
// /v1/models and /v1/chat/completions (and the same paths without /v1) for
// targeted config tests. chatHandler overrides the default chat response; pass
// nil to serve a plain success envelope.
func configTestServerWithChatHandler(t *testing.T, chatHandler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Normalize path: strip trailing /v1 or add /v1 prefix if missing.
		if path == "/models" {
			path = "/v1/models"
		} else if path == "/chat/completions" {
			path = "/v1/chat/completions"
		}
		switch path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "qwen", "context_length": 8192},
					{"id": "gemma4", "context_length": 4096},
				},
			})
		case "/v1/chat/completions":
			if chatHandler != nil {
				chatHandler(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]string{"content": `{"ok":true}`}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// writeConfigToTempDir creates a minimal config in a temp directory and returns
// a cleanup function that restores XDG_CONFIG_HOME.
func writeConfigToTempDir(t *testing.T, configBody string) (string, func()) {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	return root, func() { t.Setenv("XDG_CONFIG_HOME", oldXDG) }
}

func TestConfigModelUpdatesModel(t *testing.T) {
	server := configTestServer(t)
	defer server.Close()

	configBody := fmt.Sprintf(`[llm]
provider='openai-compatible'
base_url='%s'
model='qwen'
api_key=''
timeout='45s'
response_mode='json_object'
max_requests=32
`, server.URL)
	_, restore := writeConfigToTempDir(t, configBody)
	defer restore()

	// Change model from qwen to gemma4.
	captureStdoutStderr(t, func() {
		if got := run([]string{"config", "--model", "gemma4"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
	})

	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Model != "gemma4" {
		t.Fatalf("model = %q, want gemma4", cfg.LLM.Model)
	}
	// Response mode should be refreshed.
	if cfg.LLM.ResponseMode != "json_schema" {
		t.Fatalf("response_mode = %q, want json_schema", cfg.LLM.ResponseMode)
	}
}

func TestConfigModelWithAbsentIDFails(t *testing.T) {
	server := configTestServer(t)
	defer server.Close()

	configBody := fmt.Sprintf(`[llm]
provider='openai-compatible'
base_url='%s'
model='qwen'
api_key=''
timeout='45s'
response_mode='json_object'
max_requests=32
`, server.URL)
	_, restore := writeConfigToTempDir(t, configBody)
	defer restore()

	// Try to set a model that doesn't exist in the server's model list.
	_, stderr := captureStdoutStderr(t, func() {
		if got := run([]string{"config", "--model", "nonexistent"}); got != 2 {
			t.Fatalf("expected exit 2, got %d", got)
		}
	})
	if !strings.Contains(stderr, "not reported by the endpoint") && !strings.Contains(stderr, "nonexistent") {
		t.Fatalf("expected model rejection error in stderr, got: %s", stderr)
	}
}

func TestConfigFailsWithoutExistingConfig(t *testing.T) {
	// No config exists; targeted flags should direct user to --wizard.
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)

	_, stderr := captureStdoutStderr(t, func() {
		if got := run([]string{"config", "--model", "gemma4"}); got != 2 {
			t.Fatalf("expected exit 2, got %d", got)
		}
	})
	if !strings.Contains(stderr, "--wizard") {
		t.Fatalf("expected --wizard hint in stderr, got: %s", stderr)
	}
}

func TestConfigModelRefreshesResponseMode(t *testing.T) {
	// Reuse the shared config server, overriding only the chat handler to reject
	// json_schema while accepting json_object, to trigger response_mode refresh.
	server := configTestServerWithChatHandler(t, func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Reject json_schema but accept json_object.
		if rf, ok := request["response_format"].(map[string]any); ok && rf["type"] == "json_schema" {
			http.Error(w, "json_schema unsupported", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": `{"ok":true}`}}},
		})
	})
	defer server.Close()

	configBody := fmt.Sprintf(`[llm]
provider='openai-compatible'
base_url='%s'
model='qwen'
api_key=''
timeout='45s'
response_mode='prompt_json'
max_requests=32
`, server.URL)
	_, restore := writeConfigToTempDir(t, configBody)
	defer restore()

	captureStdoutStderr(t, func() {
		if got := run([]string{"config", "--model", "qwen"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
	})

	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Should have upgraded from prompt_json to json_object.
	if cfg.LLM.ResponseMode != "json_object" {
		t.Fatalf("response_mode = %q, want json_object", cfg.LLM.ResponseMode)
	}
}

func TestConfigCodeModelSetsValue(t *testing.T) {
	server := configTestServer(t)
	defer server.Close()

	configBody := fmt.Sprintf(`[llm]
provider='openai-compatible'
base_url='%s'
model='qwen'
api_key=''
timeout='45s'
response_mode='json_object'
max_requests=32
`, server.URL)
	_, restore := writeConfigToTempDir(t, configBody)
	defer restore()

	captureStdoutStderr(t, func() {
		if got := run([]string{"config", "--code-model", "qwen-coder"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
	})

	cfg, err := config.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.CodeModel != "qwen-coder" {
		t.Fatalf("code_model = %q, want qwen-coder", cfg.LLM.CodeModel)
	}
	if cfg.LLM.Model != "qwen" {
		t.Fatalf("model should be unchanged, got %q", cfg.LLM.Model)
	}
}
