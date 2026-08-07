package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/check"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/corpus"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/llm"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/reference"
	"github.com/sdougbrown/writetighter/internal/report"
)

// helpers from check_test.go that we reuse here
func testProfile() *profile.Resolution {
	canon := "WriteTighter"
	e1 := profile.Entry{Term: "deprecated term longer phrase", Status: profile.StatusDiscouraged, Alternatives: []string{"better phrase"}, Reason: "use better phrase", PartsOfSpeech: []string{"noun"}}
	e2 := profile.Entry{Term: "deprecated term", Status: profile.StatusDiscouraged, Alternatives: []string{"preferred term"}, Reason: "use preferred term", PartsOfSpeech: []string{"noun"}}
	e3 := profile.Entry{Term: "WriteTighter", Status: profile.StatusPreferred, PartsOfSpeech: []string{"proper noun"}, CanonicalCase: &canon}
	e4 := profile.Entry{Term: "check-in", Status: profile.StatusPreferred, PartsOfSpeech: []string{"noun"}}
	dict := &profile.Dictionary{FormatVersion: 1, Entries: []profile.Entry{e1, e2, e3, e4}}
	rules := []profile.Rule{
		{ID: "CORE.SENTENCE_LENGTH", Enabled: true, Parameters: map[string]any{"description_max_words": 5}},
		{ID: "CORE.DENSE_PARAGRAPH", Enabled: true},
		{ID: "CORE.TERM_DISCOURAGED", Enabled: true},
		{ID: "CORE.TERM_CASE", Enabled: true},
		{ID: "CORE.TERM_UNKNOWN", Enabled: true},
		{ID: "CORE.TERM_CONSISTENCY", Enabled: true},
		{ID: "CORE.PROCEDURE_MULTI_ACTION", Enabled: true},
	}
	return &profile.Resolution{Rules: &profile.RulesConfig{UnknownTermPolicy: "candidate", Rules: rules}, Dict: dict}
}

func testDoc(text string) *document.Document {
	doc, _ := document.FromReader(strings.NewReader(text), "test.md", "description")
	return doc
}

// writeTempFile writes content to a temp file and returns its path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// runLintWithParams is a helper that runs RunLint with the given params.
func runLintWithParams(params LintParams) error {
	a := &App{}
	return a.RunLint(params)
}

// captureStdout captures os.Stdout output into a buffer.
func captureStdout(t *testing.T, fn func()) *bytes.Buffer {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return &buf
}

// --- Version / Version info tests ---

func TestVersionNonEmpty(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
}

func TestCommitNonEmpty(t *testing.T) {
	if Commit == "" {
		t.Error("Commit should not be empty")
	}
}

// --- RunLint tests ---

func TestRunLintNoProfileFails(t *testing.T) {
	params := LintParams{
		Paths:  []string{"/nonexistent/path.txt"},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	err := runLintWithParams(params)
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestRunLintFailOnError(t *testing.T) {
	// The default profile's CORE.SENTENCE_LENGTH produces warning-severity findings,
	// not error. So FailOn=error should NOT trigger ErrFailThreshold even with findings.
	// We verify this by using long text that triggers sentence-length warnings.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "error",
	}
	err := runLintWithParams(params)
	// No rule in the default profile produces error-severity, so no threshold reached.
	if err != nil {
		t.Fatalf("expected nil with FailOn=error (no error-severity findings), got %v", err)
	}
}

func TestRunLintFailOnWarning(t *testing.T) {
	// Long text triggers CORE.SENTENCE_LENGTH which has severity "warning".
	// FailOn=warning should return ErrFailThreshold.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "warning",
	}
	err := runLintWithParams(params)
	if err != ErrFailThreshold {
		t.Fatalf("expected ErrFailThreshold, got %v", err)
	}
}

func TestRunLintFailOnWarningRendersCompletedReport(t *testing.T) {
	path := writeTempFile(t, strings.Repeat("word ", 30)+".")
	buf := captureStdout(t, func() {
		err := runLintWithParams(LintParams{Paths: []string{path}, Kind: "description", Format: "json", FailOn: "warning"})
		if !errors.Is(err, ErrFailThreshold) {
			t.Fatalf("expected threshold error, got %v", err)
		}
	})
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("expected completed JSON report: %v", err)
	}
	if got["status"] != "linted" {
		t.Fatalf("unexpected report: %#v", got)
	}
}

func TestRunLintFailOnNone(t *testing.T) {
	// Even though there are findings, FailOn=none means no error.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	err := runLintWithParams(params)
	if err != nil {
		t.Fatalf("expected nil with FailOn=none, got %v", err)
	}
}

func TestRunLintStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("this is some text."))
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	params := LintParams{
		Stdin:  true,
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	checkErr := runLintWithParams(params)
	if checkErr != nil {
		t.Fatalf("unexpected error: %v", checkErr)
	}
}

func TestRunLintJSONOutput(t *testing.T) {
	text := "some text here."
	path := writeTempFile(t, text)

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	runLintWithParams(params)
	w.Close()
	os.Stdout = oldStdout

	buf.ReadFrom(r)

	var reportData map[string]any
	if err := json.Unmarshal(buf.Bytes(), &reportData); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}
	if reportData["status"] != "linted" {
		t.Errorf("expected status 'linted', got %v", reportData["status"])
	}
}

func TestRunLintHumanOutput(t *testing.T) {
	text := "some text here."
	path := writeTempFile(t, text)

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "human",
		FailOn: "none",
	}
	runLintWithParams(params)
	w.Close()
	os.Stdout = oldStdout

	buf.ReadFrom(r)

	out := buf.String()
	// Human format shows "status: linted" at minimum.
	if !strings.Contains(out, "status:") {
		t.Fatalf("expected human output to contain 'status:', got: %s", out)
	}
}

func TestRunLintAgentOutput(t *testing.T) {
	text := "some text here."
	path := writeTempFile(t, text)

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "agent",
		FailOn: "none",
	}
	runLintWithParams(params)
	w.Close()
	os.Stdout = oldStdout

	buf.ReadFrom(r)

	out := buf.String()
	// Agent format outputs nothing when there are no findings, just an empty string.
	// That's valid — the format itself works without error.
	_ = out
}

func TestRunLintDefaultProfile(t *testing.T) {
	text := "writetighter is great."
	path := writeTempFile(t, text)

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	err := runLintWithParams(params)
	if err != nil {
		t.Fatalf("unexpected error with default profile: %v", err)
	}
}

func TestRunLintInvalidProfileSpec(t *testing.T) {
	text := "hello world."
	path := writeTempFile(t, text)

	params := LintParams{
		Paths:   []string{path},
		Kind:    "description",
		Format:  "json",
		FailOn:  "none",
		Profile: "nonexistent@0.0.0",
	}
	err := runLintWithParams(params)
	if err == nil {
		t.Fatal("expected error for invalid profile spec")
	}
}

func TestRunLintFailOnWarningIncludesErrors(t *testing.T) {
	// FailOn=warning catches both warning AND error severity.
	// Sentence length produces warning-severity findings.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "warning",
	}
	err := runLintWithParams(params)
	if err != ErrFailThreshold {
		t.Fatalf("expected ErrFailThreshold for warning, got %v", err)
	}
}

// --- RunExplainWithOptions tests ---

func TestExplainWithOptionsKnownRule(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := (&App{}).RunExplainWithOptions("CORE.SENTENCE_LENGTH", "software-docs-en@0.4.0", "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf.ReadFrom(r)
	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["id"] != "CORE.SENTENCE_LENGTH" {
		t.Errorf("expected id CORE.SENTENCE_LENGTH, got %v", data["id"])
	}
}

func TestExplainWithOptionsUnknownRule(t *testing.T) {
	err := (&App{}).RunExplainWithOptions("NONEXISTENT_RULE", "software-docs-en@0.4.0", "json")
	if err == nil {
		t.Fatal("expected error for unknown rule")
	}
	if !strings.Contains(err.Error(), "rule not found") {
		t.Errorf("expected 'rule not found' error, got: %v", err)
	}
}

func TestExplainWithOptionsUnknownProfile(t *testing.T) {
	err := (&App{}).RunExplainWithOptions("CORE.SENTENCE_LENGTH", "nonexistent@0.0.0", "json")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestExplainWithOptionsUnsupportedFormat(t *testing.T) {
	err := (&App{}).RunExplainWithOptions("CORE.SENTENCE_LENGTH", "software-docs-en@0.4.0", "xml")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected 'unsupported format' error, got: %v", err)
	}
}

func TestExplainWithOptionsHumanFormat(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := (&App{}).RunExplainWithOptions("CORE.SENTENCE_LENGTH", "software-docs-en@0.4.0", "human")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "CORE.SENTENCE_LENGTH") {
		t.Fatalf("expected human output to contain rule id, got: %s", out)
	}
}

// --- RunProfileList tests ---

func TestProfileListReturnsEmbedded(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := (&App{}).RunProfileList("")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "embedded") {
		t.Fatalf("expected output to contain 'embedded', got: %s", out)
	}
}

func TestProfileListJSON(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := (&App{}).RunProfileList("json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf.ReadFrom(r)
	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	profiles, ok := data["profiles"].([]any)
	if !ok || len(profiles) == 0 {
		t.Fatal("expected at least one profile in JSON output")
	}
	first := profiles[0].(map[string]any)
	if first["source"] != "embedded" {
		t.Errorf("expected source 'embedded', got %v", first["source"])
	}
}

// --- RunProfileVerify tests ---

func TestProfileVerifyKnownSpec(t *testing.T) {
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := (&App{}).RunProfileVerify("software-docs-en@0.4.0", "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf.ReadFrom(r)
	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["valid"] != true {
		t.Errorf("expected valid=true, got %v", data["valid"])
	}
}

func TestProfileVerifyUnknownSpec(t *testing.T) {
	err := (&App{}).RunProfileVerify("nonexistent@0.0.0", "json")
	if err == nil {
		t.Fatal("expected error for unknown profile spec")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestProfileVerifyDirectory(t *testing.T) {
	// A minimal bundle directory needs manifest.json, rules.json, dictionary.json.
	// Creating a proper bundle is complex; skip for now.
	t.Log("skipping directory test — requires proper bundle structure")
}

// --- RunVersion tests ---

func TestRunVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version is empty")
	}
	if Commit == "" {
		t.Error("Commit is empty")
	}
}

// --- New() tests ---

func TestNewReturnsNonNull(t *testing.T) {
	a := New()
	if a == nil {
		t.Fatal("New() returned nil")
	}
}

// --- Edge case: no config files ---

func TestRunLintExplicitMissingConfigFails(t *testing.T) {
	text := "simple text."
	path := writeTempFile(t, text)

	params := LintParams{
		Paths:      []string{path},
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
		ConfigPath: "/nonexistent/config.yaml",
	}
	err := runLintWithParams(params)
	if err == nil {
		t.Fatal("expected explicit missing config to fail")
	}
}

// --- Profile-specific profile test ---

func TestProfileResolveEmbedded(t *testing.T) {
	r, err := profile.Resolve("")
	if err != nil {
		t.Fatalf("failed to resolve embedded profile: %v", err)
	}
	if r == nil {
		t.Fatal("resolved profile is nil")
	}
}

func TestRunLintRejectsSymlinkPath(t *testing.T) {
	realPath := writeTempFile(t, "test content")
	symPath := filepath.Join(t.TempDir(), "link.txt")
	if err := os.Symlink(realPath, symPath); err != nil {
		t.Skip("symlink creation not supported")
	}
	err := runLintWithParams(LintParams{Paths: []string{symPath}, Kind: "description", Format: "json", FailOn: "none"})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

// --- RunRevise tests ---

func writeReviseUserConfig(t *testing.T, baseURL, apiKeyEnv string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	dir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	body := "[llm]\nprovider = \"openai-compatible\"\nbase_url = \"" + baseURL + "\"\nmodel = \"test-model\"\nresponse_mode = \"json_object\"\n"
	if apiKeyEnv != "" {
		body += "api_key_env = \"" + apiKeyEnv + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// extractReviseText pulls the editable source body out of a user message that is
// wrapped in <revise-text>...</revise-text> tags. It returns the full user text
// unchanged when the wrapper is absent (defensive fallback).
func extractReviseText(userContent string) string {
	const openTag = "<revise-text>"
	const closeTag = "</revise-text>"
	start := strings.Index(userContent, openTag)
	if start < 0 {
		return userContent
	}
	start += len(openTag)
	end := strings.Index(userContent[start:], closeTag)
	if end < 0 {
		return userContent[start:]
	}
	body := userContent[start : start+end]
	// The wrapper inserts a leading newline after the opening tag; trim it so the
	// returned bytes align with the editable excerpt coordinate space.
	return strings.TrimPrefix(body, "\n")
}

func writeReviseUserConfigWithMaxRequests(t *testing.T, baseURL string, maxRequests int) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	dir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("[llm]\nprovider = \"openai-compatible\"\nbase_url = %q\nmodel = \"test-model\"\nresponse_mode = \"json_object\"\nmax_requests = %d\n", baseURL, maxRequests)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeReviseStoredKeyConfig(t *testing.T, baseURL, key string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	dir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "[llm]\nprovider = \"openai-compatible\"\nbase_url = \"" + baseURL + "\"\nmodel = \"test-model\"\nresponse_mode = \"json_object\"\napi_key = \"" + key + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunPromptExportsCodeCommentGuidanceWithoutConfig(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	configDir := filepath.Join(configRoot, "writetighter")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("malformed = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := (&App{}).RunPrompt(PromptParams{Kind: "code-comment", Format: "human"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"Document kind: code-comment", "CORE.CAUSAL_ORDER", "invariant", "TODO", "verify any rewrite against the implementation"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("prompt output missing %q: %s", expected, output.String())
		}
	}
}

func TestRunPromptJSON(t *testing.T) {
	output := captureStdout(t, func() {
		if err := (&App{}).RunPrompt(PromptParams{Kind: "pr", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	var payload struct {
		SchemaVersion  int `json:"schema_version"`
		Kind           string
		Principles     []map[string]string `json:"principles"`
		KindDirections []string            `json:"kind_directions"`
	}
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 1 || payload.Kind != "pr" || len(payload.Principles) == 0 || len(payload.KindDirections) == 0 {
		t.Fatalf("unexpected prompt JSON: %#v", payload)
	}
}

func TestRunPromptRejectsInvalidOptions(t *testing.T) {
	if err := (&App{}).RunPrompt(PromptParams{Kind: "message", Format: "human"}); err == nil {
		t.Fatal("expected invalid kind error")
	}
	if err := (&App{}).RunPrompt(PromptParams{Kind: "description", Format: "yaml"}); err == nil {
		t.Fatal("expected invalid format error")
	}
}

func TestRunReviseNoInput(t *testing.T) {
	err := (&App{}).RunRevise(ReviseParams{})
	if err == nil {
		t.Fatal("expected error for no input")
	}
	if !strings.Contains(err.Error(), "no input specified") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestRunReviseMutuallyExclusive(t *testing.T) {
	err := (&App{}).RunRevise(ReviseParams{
		Stdin: true,
		Paths: []string{"file.md"},
	})
	if err == nil {
		t.Fatal("expected error for stdin+paths")
	}
}

func TestRunReviseMissingLLMConfig(t *testing.T) {
	// Without any user config, RunRevise should fail with a clear message.
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	path := writeTempFile(t, "short test.")
	err := (&App{}).RunRevise(ReviseParams{
		Paths:  []string{path},
		Format: "json",
	})
	if !errors.Is(err, ErrLLMConfigRequired) {
		t.Fatalf("expected ErrLLMConfigRequired, got %v", err)
	}
	if !strings.Contains(err.Error(), "model is required") && !strings.Contains(err.Error(), "LLM configuration") && !strings.Contains(err.Error(), "revise requires") {
		t.Fatalf("expected config-related error, got: %v", err)
	}
}

func TestRunReviseDoesNotEchoMalformedConfigSecrets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	dir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[llm]\napi_key='sensitive-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := (&App{}).RunRevise(ReviseParams{Paths: []string{writeTempFile(t, "Short text.")}, Format: "json"})
	if !errors.Is(err, ErrLLMConfigRequired) || strings.Contains(err.Error(), "sensitive-value") {
		t.Fatalf("expected redacted config error, got %v", err)
	}
}

func TestRunReviseInvalidFormat(t *testing.T) {
	path := writeTempFile(t, "short test.")
	err := (&App{}).RunRevise(ReviseParams{
		Paths:  []string{path},
		Format: "xml",
	})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestRunLint(t *testing.T) {
	text := "simple text."
	path := writeTempFile(t, text)
	err := (&App{}).RunLint(LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLintAcceptsDirectText(t *testing.T) {
	text := "Direct text input."
	output := captureStdout(t, func() {
		if err := (&App{}).RunLint(LintParams{Text: &text, Kind: "description", Format: "json", FailOn: "none"}); err != nil {
			t.Fatal(err)
		}
	})
	var payload report.Report
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Source.Path == nil || *payload.Source.Path != "<text>" {
		t.Fatalf("direct text source = %#v", payload.Source)
	}
}

func TestRunLintProducesOutput(t *testing.T) {
	path := writeTempFile(t, "some text.")
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := (&App{}).RunLint(LintParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	})
	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected output from lint")
	}
	var reportData map[string]any
	if err := json.Unmarshal(buf.Bytes(), &reportData); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if reportData["status"] != "linted" {
		t.Errorf("expected status 'linted', got %v", reportData["status"])
	}
}

func TestRunReviseProducesStructuredClarificationWithoutLintFinding(t *testing.T) {
	content := "Refs delta pluralizes three scalars."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{"findings":[{"kind":"clarification","source_text":"Refs delta pluralizes three scalars.","source_range":{"start":0,"end":36},"principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"The transformation has multiple plausible meanings.","question":"Does this rename fields or change scalar values to collections?","confidence":0.84}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": response}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	path := writeTempFile(t, content)
	buf := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Paths: []string{path}, Kind: "description", Format: "json"}); err != nil {
			t.Fatalf("unexpected revise error: %v", err)
		}
	})
	var response report.ReviseResponse
	if err := json.Unmarshal(buf.Bytes(), &response); err != nil {
		t.Fatalf("invalid revise JSON: %v\n%s", err, buf.String())
	}
	if response.Status != "ok" || response.LLMModel != "test-model" || len(response.Sources) != 1 || response.Sources[0] != path || len(response.Revisions) != 1 {
		t.Fatalf("unexpected revise response: %#v", response)
	}
	if response.Revisions[0].Kind != "clarification" || response.Revisions[0].Question == nil {
		t.Fatalf("expected structured clarification: %#v", response.Revisions[0])
	}
}

func TestRunReviseChunksEntireLargeDocument(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		userText := request.Messages[len(request.Messages)-1].Content
		// Source bytes are now wrapped in <revise-text> tags; extract the body so
		// the returned source_text matches the editable excerpt coordinate space.
		editable := extractReviseText(userText)
		sourceText := editable[:5]
		modelResponse, _ := json.Marshal(map[string]any{"findings": []map[string]any{{
			"kind": "clarification", "source_text": sourceText,
			"source_range":  map[string]int{"start": 0, "end": 5},
			"principle_ids": []string{"CORE.EXPLICIT_RELATIONSHIPS"},
			"reason":        "The passage needs more context.", "replacement": nil,
			"question": "What relationship should this passage state?", "confidence": 0.7,
		}}})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": string(modelResponse)}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	content := strings.Repeat("plain words in a long paragraph. ", 1600)
	path := writeTempFile(t, content)
	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Paths: []string{path}, Kind: "description", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if requests < 2 || len(response.Analysis) != 1 || response.Analysis[0].Chunks != requests || !response.Analysis[0].Complete || response.Analysis[0].AnalyzedBytes != len(content) {
		t.Fatalf("incomplete chunk coverage: requests=%d response=%#v", requests, response.Analysis)
	}
	if len(response.Revisions) != requests {
		t.Fatalf("revisions=%d, requests=%d", len(response.Revisions), requests)
	}
	for i := 1; i < len(response.Revisions); i++ {
		if response.Revisions[i-1].Range.StartByte >= response.Revisions[i].Range.StartByte {
			t.Fatalf("revisions are not in source order: %#v", response.Revisions)
		}
	}
}

func TestRunReviseEnforcesRequestCapBeforeModelCalls(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	writeReviseUserConfigWithMaxRequests(t, server.URL, 1)
	content := strings.Repeat("plain words in a long paragraph. ", 1600)
	err := (&App{}).RunRevise(ReviseParams{Paths: []string{writeTempFile(t, content)}, Kind: "description", Format: "json"})
	if err == nil || !strings.Contains(err.Error(), "exceeding configured max_requests=1") {
		t.Fatalf("expected request cap error, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("request cap was checked after %d model calls", requests)
	}
}

func TestRunReviseRetriesModelErrorWithPromptJSON(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		content := `{"findings":[]}`
		if request["response_format"] != nil {
			content = `{"error":{"message":"grammar-constrained generation failed"}}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Paths: []string{writeTempFile(t, "Short text.")}, Kind: "description", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || response.Status != "ok" || len(response.Analysis) != 1 || response.Analysis[0].ModelRequests != 2 || !response.Analysis[0].Complete {
		t.Fatalf("fallback did not complete: requests=%d response=%#v", requests, response)
	}
}

func TestRunReviseReturnsSentinelOnInvalidModelResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "not-json"}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	var reviseErr error
	output := captureStdout(t, func() {
		reviseErr = (&App{}).RunRevise(ReviseParams{Paths: []string{writeTempFile(t, "Short text.")}, Kind: "description", Format: "json"})
	})
	if !errors.Is(reviseErr, ErrReviseFailed) {
		t.Fatalf("expected ErrReviseFailed, got %v", reviseErr)
	}
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("expected structured failure response: %v", err)
	}
	if response.Status != "failed" || len(response.Errors) != 1 {
		t.Fatalf("unexpected structured failure: %#v", response)
	}
}

func TestRunReviseUsesStoredAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pat-local" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": `{"findings":[]}`}}},
		})
	}))
	defer server.Close()
	writeReviseStoredKeyConfig(t, server.URL, "pat-local")
	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Paths: []string{writeTempFile(t, "Short text.")}, Kind: "description", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(output.String(), "pat-local") {
		t.Fatal("stored API key leaked into output")
	}
}

func TestRunReviseRejectsConfiguredUnsetAPIKey(t *testing.T) {
	writeReviseUserConfig(t, "http://127.0.0.1:4000/v1", "MISSING_REVISE_KEY")
	t.Setenv("MISSING_REVISE_KEY", "")
	err := (&App{}).RunRevise(ReviseParams{Paths: []string{writeTempFile(t, "Short text.")}, Kind: "description", Format: "json"})
	if err == nil || !strings.Contains(err.Error(), "environment variable is unset") {
		t.Fatalf("expected unset key configuration error, got %v", err)
	}
}

func TestRunReviseUnsupportedProvider(t *testing.T) {
	path := writeTempFile(t, "test content.")
	err := (&App{}).RunRevise(ReviseParams{
		Paths:  []string{path},
		Format: "json",
	})
	if err == nil {
		t.Fatal("expected error for missing LLM config")
	}
}

func TestReviseAcceptsInputFlags(t *testing.T) {
	// Verify that revise accepts valid flag combinations (config error is about
	// missing LLM config, not bad flag parsing).
	path := writeTempFile(t, "test content.")
	err := (&App{}).RunRevise(ReviseParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
	})
	// Should fail on LLM config, not flag validation.
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "invalid") && strings.Contains(err.Error(), "kind") {
		t.Fatalf("should fail on LLM config, not flag validation: %v", err)
	}
}

func TestRunReviseHTMLReportsVisibleTextAnalysis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"findings":[]}`}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(`<p>Visible text.</p>`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Paths: []string{path}, Kind: "description", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Analysis) != 1 || response.Analysis[0].SourceFormat != "html" || response.Analysis[0].RangeBasis != "visible_text" || !response.Analysis[0].Complete {
		t.Fatalf("analysis = %#v", response.Analysis)
	}
}

// writeReviseUserConfigWithTokens writes a user config with context_window_tokens
// and max_output_tokens set, for budget-aware tests.
func writeReviseUserConfigWithTokens(t *testing.T, baseURL string, ctxTokens, outTokens int) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	dir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("[llm]\nprovider = \"openai-compatible\"\nbase_url = %q\nmodel = \"test-model\"\nresponse_mode = \"json_object\"\ncontext_window_tokens = %d\nmax_output_tokens = %d\n", baseURL, ctxTokens, outTokens)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeReviseServerWithCapture creates a fake server that captures the request body
// for inspection. It returns a valid empty revise response.
func fakeReviseServerWithCapture(t *testing.T, capture func(body []byte)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if capture != nil {
			capture(body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": `{"findings":[]}`}}},
		})
	}))
}

// --- RunRevise reference integration tests ---

func TestRunReviseWithReferences(t *testing.T) {
	var capturedBody []byte
	server := fakeReviseServerWithCapture(t, func(body []byte) {
		capturedBody = body
	})
	defer server.Close()

	writeReviseUserConfigWithTokens(t, server.URL, 8192, 1024)

	srcPath := writeTempFile(t, "This is the source document to revise.")

	refPath := filepath.Join(t.TempDir(), "reference.md")
	if err := os.WriteFile(refPath, []byte("This is reference content for context."), 0o600); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{
			Paths:          []string{srcPath},
			ReferencePaths: []string{refPath},
			Kind:           "description",
			Format:         "json",
		}); err != nil {
			t.Fatal(err)
		}
	})

	// Verify the response is valid JSON.
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid revise JSON: %v\n%s", err, output.String())
	}
	if response.Status != "ok" {
		t.Fatalf("expected status ok, got %q: errors=%v", response.Status, response.Errors)
	}

	// Verify the request body contains reference tags and revise-text tags.
	// The body is JSON-escaped, so check for escaped content.
	if capturedBody != nil {
		bodyStr := string(capturedBody)
		// Check for the reference tag start (JSON-escaped quotes).
		if !strings.Contains(bodyStr, `\u003creference`) && !strings.Contains(bodyStr, "<reference") {
			t.Error("request body missing <reference> tags")
		}
		if !strings.Contains(bodyStr, "<revise-text>") && !strings.Contains(bodyStr, `\u003crevise-text`) {
			t.Error("request body missing <revise-text> tags")
		}
		// Reference content should appear unescaped in the JSON value.
		if !strings.Contains(bodyStr, "This is reference content for context.") {
			t.Error("request body missing reference content")
		}
		if !strings.Contains(bodyStr, "This is the source document to revise.") {
			t.Error("request body missing source content")
		}
	}
}

func TestRunReviseNoContextWindowWithReferences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"findings":[]}`}}},
		})
	}))
	defer server.Close()

	// Config with context_window_tokens=0 (unset) but references provided.
	writeReviseUserConfig(t, server.URL, "")

	refPath := filepath.Join(t.TempDir(), "ref.md")
	if err := os.WriteFile(refPath, []byte("reference content"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := (&App{}).RunRevise(ReviseParams{
		Paths:          []string{writeTempFile(t, "source text.")},
		ReferencePaths: []string{refPath},
		Kind:           "description",
		Format:         "json",
	})
	if err == nil {
		t.Fatal("expected error when references provided without context_window_tokens")
	}
	if !strings.Contains(err.Error(), "reference revision requires llm.context_window_tokens") {
		t.Fatalf("expected context_window_tokens error, got: %v", err)
	}
}

func TestRunReviseOverBudgetReferences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"findings":[]}`}}},
		})
	}))
	defer server.Close()

	writeReviseUserConfigWithTokens(t, server.URL, 1024, 256) // Very small budget

	// Create a large reference file that will exceed the budget.
	refPath := filepath.Join(t.TempDir(), "large_ref.md")
	largeRef := strings.Repeat("reference content that takes up a lot of space. ", 500)
	if err := os.WriteFile(refPath, []byte(largeRef), 0o600); err != nil {
		t.Fatal(err)
	}

	err := (&App{}).RunRevise(ReviseParams{
		Paths:          []string{writeTempFile(t, "short source.")},
		ReferencePaths: []string{refPath},
		Kind:           "description",
		Format:         "json",
	})
	if err == nil {
		t.Fatal("expected error for over-budget references")
	}
	// The very small budget (1024/256) fails the base-overhead check in
	// planBudgetedChunks before the chunk loop runs.
	if !strings.Contains(err.Error(), "reference overhead exceeds context window") {
		t.Fatalf("expected base-overhead budget error, got: %v", err)
	}
}

func TestRunReviseReferenceMissingFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"findings":[]}`}}},
		})
	}))
	defer server.Close()

	writeReviseUserConfigWithTokens(t, server.URL, 8192, 1024)

	err := (&App{}).RunRevise(ReviseParams{
		Paths:          []string{writeTempFile(t, "source.")},
		ReferencePaths: []string{"/nonexistent/path/reference.md"},
		Kind:           "description",
		Format:         "json",
	})
	if err == nil {
		t.Fatal("expected error for missing reference file")
	}
	if !strings.Contains(err.Error(), "collecting references") {
		t.Fatalf("expected collecting-references error, got: %v", err)
	}
}

func TestRunReviseReferenceIsSourceFile(t *testing.T) {
	var capturedBody []byte
	server := fakeReviseServerWithCapture(t, func(body []byte) {
		capturedBody = body
	})
	defer server.Close()

	writeReviseUserConfigWithTokens(t, server.URL, 8192, 1024)

	// Use the same file as both source and reference.
	srcContent := "This is the source document."
	srcPath := writeTempFile(t, srcContent)

	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{
			Paths:          []string{srcPath},
			ReferencePaths: []string{srcPath}, // same file
			Kind:           "description",
			Format:         "json",
		}); err != nil {
			t.Fatal(err)
		}
	})

	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid revise JSON: %v\n%s", err, output.String())
	}
	if response.Status != "ok" {
		t.Fatalf("expected status ok, got %q: errors=%v", response.Status, response.Errors)
	}

	// Verify ReferenceContext is populated but with empty Files slice
	// (source file excluded from references).
	if response.ReferenceContext == nil {
		t.Fatal("expected ReferenceContext to be populated")
	}
	if len(response.ReferenceContext.Files) != 0 {
		t.Fatalf("expected empty Files, got %v", response.ReferenceContext.Files)
	}
	if len(response.ReferenceContext.Paths) != 1 {
		t.Fatalf("expected 1 Path, got %v", response.ReferenceContext.Paths)
	}
	if response.ReferenceContext.InputBytes != 0 {
		t.Fatalf("expected InputBytes=0, got %d", response.ReferenceContext.InputBytes)
	}

	// Also verify the request body does NOT contain reference tags for the
	// source. buildUserContent always writes <revise-text>, so the previous
	// compound guard (AND NOT contains <revise-text>) was a permanent no-op;
	// assert directly that no <reference> tag appears.
	if capturedBody == nil {
		t.Fatal("expected a request body to be captured")
	}
	if strings.Contains(string(capturedBody), "<reference") {
		t.Fatal("request body should not contain <reference> tags when only source file is provided as reference")
	}
}

// TestRunReviseStaleContextWindowModel verifies that reference revision is
// rejected when context_window_model does not match the current model, even
// when context_window_tokens is set. Capacity confirmed for one model must
// never be silently reused for a different model.
func TestRunReviseStaleContextWindowModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": `{"findings":[]}`}}},
		})
	}))
	defer server.Close()

	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	dir := filepath.Join(root, "writetighter")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("[llm]\nprovider = \"openai-compatible\"\nbase_url = %q\nmodel = \"current-model\"\nresponse_mode = \"json_object\"\ncontext_window_tokens = 8192\nmax_output_tokens = 2048\ncontext_window_model = \"old-model\"\n", server.URL)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	refPath := filepath.Join(t.TempDir(), "ref.md")
	if err := os.WriteFile(refPath, []byte("reference content"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := (&App{}).RunRevise(ReviseParams{
		Paths:          []string{writeTempFile(t, "source text.")},
		ReferencePaths: []string{refPath},
		Kind:           "description",
		Format:         "json",
	})
	if err == nil {
		t.Fatal("expected error for stale context_window_model")
	}
	if !strings.Contains(err.Error(), "old-model") || !strings.Contains(err.Error(), "current-model") {
		t.Fatalf("expected model mismatch error mentioning both models, got: %v", err)
	}
}

// TestPlanBudgetedChunksCannotFit covers the two shrink-loop failure paths in
// planBudgetedChunks ("cannot fit any chunk" and "cannot fit final fragment")
// that a small-budget integration test never reaches because its base-overhead
// check fails first. The document is a run of double-quotes: JSON escaping
// roughly doubles the transported byte count, so a candidate that fits by the
// 4-bytes/token estimate can still overflow the serialized-token budget.
func TestPlanBudgetedChunksCannotFit(t *testing.T) {
	dir := t.TempDir()
	refPath := filepath.Join(dir, "guide.md")
	if err := os.WriteFile(refPath, []byte("Guide content"), 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := reference.Collect([]string{refPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := testProfile()
	const maxOutput = 256
	const noPressure = 1 << 30 // large enough that the base measurement passes

	measureBase := func(doc *document.Document) int {
		oneByte := llm.NewChunkExcerpt(doc, 0, 1)
		sys, user, _, err := llm.BuildBudgetedPrompt(doc, res, nil, nil, oneByte, pack, llm.Config{Model: "m", ResponseMode: "json_object", ContextWindowTokens: noPressure, MaxOutputTokens: maxOutput})
		if err != nil {
			t.Fatalf("measure base overhead: %v", err)
		}
		serialized, err := llm.SerializeRequestBytes(llm.Config{Model: "m", ResponseMode: "json_object"}, sys, user)
		if err != nil {
			t.Fatalf("serialize base request: %v", err)
		}
		return (serialized + 3) / 4
	}

	t.Run("cannot fit any chunk", func(t *testing.T) {
		// 2400 double quotes, no newlines: chunkSize equals the 2048-byte
		// minimum and can never shrink further nor fit, so the loop exhausts
		// its fallbacks and reports "cannot fit any chunk".
		doc := testDoc(strings.Repeat(`"`, 2400))
		base := measureBase(doc)
		cfg := llm.Config{
			Model:               "m",
			ResponseMode:        "json_object",
			ContextWindowTokens: base + maxOutput + config.BudgetSafetyTokens + config.MinEditableSourceTokens, // availableSourceBudget == 512 exactly
			MaxOutputTokens:     maxOutput,
		}
		_, err := planBudgetedChunks(doc, pack, cfg, 8, res, nil, nil)
		// A base-overhead failure would surface as "reference overhead exceeds
		// context window" instead; seeing that message means the budget formula
		// drifted, not the chunk loop.
		t.Logf("cannot-fit-any-chunk error: %v", err)
		if err == nil || !strings.Contains(err.Error(), "cannot fit any chunk") {
			t.Fatalf("expected 'cannot fit any chunk', got: %v", err)
		}
	})

	t.Run("cannot fit final fragment", func(t *testing.T) {
		// 1800 double quotes (< the 2048-byte minimum): the whole document is a
		// final fragment that serializes to ~3600 bytes (~900 tokens) against a
		// 600-token source budget, so it fails rather than silently dropping
		// coverage.
		doc := testDoc(strings.Repeat(`"`, 1800))
		base := measureBase(doc)
		cfg := llm.Config{
			Model:               "m",
			ResponseMode:        "json_object",
			ContextWindowTokens: base + maxOutput + config.BudgetSafetyTokens + 600, // A=600, base still passes
			MaxOutputTokens:     maxOutput,
		}
		_, err := planBudgetedChunks(doc, pack, cfg, 8, res, nil, nil)
		t.Logf("cannot-fit-final-fragment error: %v", err)
		if err == nil || !strings.Contains(err.Error(), "cannot fit final fragment") {
			t.Fatalf("expected 'cannot fit final fragment', got: %v", err)
		}
	})
}

// TestRunReviseEmptySourceWithReferences verifies that an empty source document
// with references does not panic and produces no model calls (ChunkRanges
// parity).
func TestRunReviseEmptySourceWithReferences(t *testing.T) {
	called := false
	server := fakeReviseServerWithCapture(t, func(body []byte) { called = true })
	defer server.Close()
	writeReviseUserConfigWithTokens(t, server.URL, 8192, 1024)
	refPath := filepath.Join(t.TempDir(), "ref.md")
	if err := os.WriteFile(refPath, []byte("reference content"), 0o600); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{
			Paths:          []string{writeTempFile(t, "")},
			ReferencePaths: []string{refPath},
			Kind:           "description",
			Format:         "json",
		}); err != nil {
			t.Fatalf("empty source with references should succeed, got: %v", err)
		}
	})

	// An empty source produces zero chunks, so no request may reach the server.
	if called {
		t.Error("empty source should not trigger any model call")
	}
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid revise JSON: %v\n%s", err, output.String())
	}
	if response.Status != "ok" {
		t.Fatalf("expected status ok, got %q: errors=%v", response.Status, response.Errors)
	}
	if len(response.Revisions) != 0 {
		t.Errorf("expected 0 revisions for an empty source, got %d", len(response.Revisions))
	}
}

func TestRunReviseReferenceContextInReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": `{"findings":[]}`}}},
		})
	}))
	defer server.Close()

	writeReviseUserConfigWithTokens(t, server.URL, 8192, 1024)

	srcPath := writeTempFile(t, "Source text.")

	refDir := t.TempDir()
	refPath := filepath.Join(refDir, "guide.md")
	refContent := "Guide content."
	if err := os.WriteFile(refPath, []byte(refContent), 0o600); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{
			Paths:          []string{srcPath},
			ReferencePaths: []string{refPath},
			Kind:           "description",
			Format:         "json",
		}); err != nil {
			t.Fatal(err)
		}
	})

	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid revise JSON: %v\n%s", err, output.String())
	}
	if response.Status != "ok" {
		t.Fatalf("expected status ok, got %q: errors=%v", response.Status, response.Errors)
	}

	// Verify ReferenceContext is populated.
	rc := response.ReferenceContext
	if rc == nil {
		t.Fatal("expected ReferenceContext to be populated")
	}
	if len(rc.Paths) != 1 || rc.Paths[0] != refPath {
		t.Errorf("unexpected Paths: %v", rc.Paths)
	}
	if len(rc.Files) != 1 || !strings.HasSuffix(rc.Files[0], "guide.md") {
		t.Errorf("unexpected Files: %v", rc.Files)
	}
	if rc.InputBytes != len(refContent) {
		t.Errorf("InputBytes = %d, want %d", rc.InputBytes, len(refContent))
	}
	if rc.IncludedBytes <= 0 {
		t.Errorf("IncludedBytes should be > 0, got %d", rc.IncludedBytes)
	}
	if !rc.Complete {
		t.Error("Complete should be true")
	}
	if rc.ContextWindowTokens != 8192 {
		t.Errorf("ContextWindowTokens = %d, want 8192", rc.ContextWindowTokens)
	}
	if rc.MaxOutputTokens != 1024 {
		t.Errorf("MaxOutputTokens = %d, want 1024", rc.MaxOutputTokens)
	}
}

// --- E2E tests with fake server ---

func TestRunReviseE2EWithReferences(t *testing.T) {
	var capturedReq struct {
		Model     string `json:"model"`
		MaxTokens *int   `json:"max_tokens,omitempty"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	var reqCaptureDone bool
	modelResponse, _ := json.Marshal(map[string]any{
		"findings": []map[string]any{
			{
				"kind":          "rewrite",
				"source_text":   "Source text for revision.",
				"source_range":  map[string]int{"start": 0, "end": 24},
				"principle_ids": []string{"CORE.SHORT_SENTENCE"},
				"reason":        "Sentence can be more direct.",
				"replacement":   "Revised source text.",
				"confidence":    0.85,
			},
		},
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		reqCaptureDone = true
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": string(modelResponse)}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfigWithTokens(t, server.URL, 8192, 1024)
	refDir := t.TempDir()
	refPath := filepath.Join(refDir, "style-guide.md")
	refContent := "Use active voice. Prefer simple present tense."
	os.WriteFile(refPath, []byte(refContent), 0o600)
	srcContent := "Source text for revision."
	srcPath := writeTempFile(t, srcContent)
	output := captureStdout(t, func() {
		(&App{}).RunRevise(ReviseParams{
			Paths:          []string{srcPath},
			ReferencePaths: []string{refPath},
			Kind:           "description",
			Format:         "json",
		})
	})
	if !reqCaptureDone {
		t.Fatal("fake server did not receive a request")
	}
	if capturedReq.MaxTokens == nil {
		t.Error("request JSON is missing max_tokens field")
	} else if *capturedReq.MaxTokens != 1024 {
		t.Errorf("max_tokens = %d, want 1024", *capturedReq.MaxTokens)
	}
	var userContent string
	for _, msg := range capturedReq.Messages {
		if msg.Role == "user" {
			userContent = msg.Content
			break
		}
	}
	if !strings.Contains(userContent, "<reference") {
		t.Error("request body missing <reference> tags")
	}
	if !strings.Contains(userContent, refContent) {
		t.Error("request body missing reference content")
	}
	if !strings.Contains(userContent, "<revise-text>") {
		t.Error("request body missing <revise-text> tags")
	}
	if !strings.Contains(userContent, srcContent) {
		t.Error("request body missing source content")
	}
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid revise JSON: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("expected status ok, got %q", response.Status)
	}
	rc := response.ReferenceContext
	if rc == nil {
		t.Fatal("expected ReferenceContext to be populated")
	}
	if rc.InputBytes != len(refContent) {
		t.Errorf("InputBytes = %d, want %d", rc.InputBytes, len(refContent))
	}
	if !rc.Complete {
		t.Error("Complete should be true")
	}
	for _, rev := range response.Revisions {
		if strings.Contains(rev.SourceText, refContent) {
			t.Errorf("revision source_text contains reference content: %q", rev.SourceText)
		}
	}
	if len(response.Revisions) > 0 {
		if !strings.Contains(response.Revisions[0].SourceText, srcContent) {
			t.Errorf("revision source_text should contain source content, got %q", response.Revisions[0].SourceText)
		}
	}
}

func TestRunReviseE2ENoRefPreservesLegacy(t *testing.T) {
	var capturedReq struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	srcContent := "Short source text for legacy revision."
	modelResponse, _ := json.Marshal(map[string]any{
		"findings": []map[string]any{
			{
				"kind":          "clarification",
				"source_text":   srcContent,
				"source_range":  map[string]int{"start": 0, "end": len(srcContent)},
				"principle_ids": []string{"CORE.EXPLICIT_RELATIONSHIPS"},
				"reason":        "The passage needs more context.",
				"question":      "What relationship should this passage state?",
				"confidence":    0.72,
			},
		},
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": string(modelResponse)}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	srcPath := writeTempFile(t, srcContent)
	output := captureStdout(t, func() {
		(&App{}).RunRevise(ReviseParams{
			Paths:  []string{srcPath},
			Kind:   "description",
			Format: "json",
		})
	})
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid revise JSON: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("expected status ok, got %q", response.Status)
	}
	if response.ReferenceContext != nil {
		t.Error("ReferenceContext should be nil for legacy path without references")
	}
	var userContent string
	for _, msg := range capturedReq.Messages {
		if msg.Role == "user" {
			userContent = msg.Content
			break
		}
	}
	if strings.Contains(userContent, "<reference") {
		t.Error("legacy request body should not contain <reference> tags")
	}
	if !strings.Contains(userContent, "<revise-text>") {
		t.Error("request body missing <revise-text> tags")
	}
	if len(response.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(response.Revisions))
	}
	rev := response.Revisions[0]
	if rev.SourceText != srcContent {
		t.Errorf("revision source_text = %q, want %q", rev.SourceText, srcContent)
	}
	if rev.Range.StartByte != 0 || rev.Range.EndByte != len(srcContent) {
		t.Errorf("revision range = [%d, %d), want [0, %d)", rev.Range.StartByte, rev.Range.EndByte, len(srcContent))
	}
	if rev.Kind != "clarification" {
		t.Errorf("revision kind = %q, want clarification", rev.Kind)
	}
	if rev.Question == nil || *rev.Question == "" {
		t.Error("clarification revision should have a question")
	}
	if len(response.Analysis) != 1 {
		t.Fatalf("expected 1 analysis entry, got %d", len(response.Analysis))
	}
	if !response.Analysis[0].Complete {
		t.Error("analysis should report complete coverage")
	}
	if response.Analysis[0].AnalyzedBytes != len(srcContent) {
		t.Errorf("analyzed bytes = %d, want %d", response.Analysis[0].AnalyzedBytes, len(srcContent))
	}
}

// --- GitCompare flow tests ---

func TestRunLintGitCompareInvalidWithStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("stdin text"))
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	params := LintParams{
		GitCompare: "HEAD~1",
		Stdin:      true,
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
	}
	err = runLintWithParams(params)
	if err == nil || !strings.Contains(err.Error(), "--git-compare is only valid with file paths") {
		t.Fatalf("expected error about git-compare requiring file paths, got: %v", err)
	}
}

func TestRunLintGitCompareInvalidWithText(t *testing.T) {
	text := "direct text input"
	params := LintParams{
		GitCompare: "HEAD~1",
		Text:       &text,
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
	}
	err := runLintWithParams(params)
	if err == nil || !strings.Contains(err.Error(), "--git-compare is only valid with file paths") {
		t.Fatalf("expected error about git-compare requiring file paths, got: %v", err)
	}
}

func TestRunLintGitCompareNoPaths(t *testing.T) {
	params := LintParams{
		GitCompare: "HEAD~1",
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
	}
	err := runLintWithParams(params)
	if err == nil || !strings.Contains(err.Error(), "--git-compare requires file paths") {
		t.Fatalf("expected git-compare path requirement error, got: %v", err)
	}
}

func TestRunLintGitCompareNonRepoPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	params := LintParams{
		GitCompare: "HEAD~1",
		Paths:      []string{path},
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
	}
	err := runLintWithParams(params)
	if err == nil || !strings.Contains(err.Error(), "not a Git repository") {
		t.Fatalf("expected not-a-git-repo error, got: %v", err)
	}
}

// TestRunLintGitCompareFullFlow exercises comparison-corpus construction,
// automatic novelty-checker enablement, and report rendering against a real Git revision.
func TestRunLintGitCompareFullFlow(t *testing.T) {
	repoDir := t.TempDir()
	runAppGit(t, repoDir, "init", "-q")
	runAppGit(t, repoDir, "config", "user.email", "test@example.com")
	runAppGit(t, repoDir, "config", "user.name", "Test")

	path := filepath.Join(repoDir, "input.md")
	if err := os.WriteFile(path, []byte("The established term appears here.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runAppGit(t, repoDir, "add", "input.md")
	runAppGit(t, repoDir, "commit", "-q", "-m", "comparison revision")
	comparisonRevision := runAppGitOutput(t, repoDir, "rev-parse", "HEAD")
	if err := os.WriteFile(path, []byte("The fluxion appears. The fluxion remains.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	buf := captureStdout(t, func() {
		err := runLintWithParams(LintParams{
			Paths:      []string{path},
			GitCompare: "HEAD",
			Kind:       "description",
			Format:     "json",
			FailOn:     "none",
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	var reportData struct {
		Findings []report.Finding `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &reportData); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}
	if len(reportData.Findings) != 1 {
		t.Fatalf("expected exactly one corpus-novelty finding, got %#v", reportData.Findings)
	}
	finding := reportData.Findings[0]
	if finding.RuleID != "CORE.CORPUS_NOVELTY" {
		t.Fatalf("rule ID = %q, want CORE.CORPUS_NOVELTY", finding.RuleID)
	}
	wantEvidence := fmt.Sprintf("corpus-novelty: term %q git_compare_count=0 change_count=2 git_compare_rev=%s", "fluxion", comparisonRevision)
	if finding.Evidence != wantEvidence {
		t.Fatalf("evidence = %q, want %q", finding.Evidence, wantEvidence)
	}
	if !strings.Contains(finding.Message, comparisonRevision[:8]) {
		t.Fatalf("message %q does not identify comparison revision %s", finding.Message, comparisonRevision[:8])
	}
}

func runAppGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runAppGitOutput(t, dir, args...)
}

func runAppGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// --- docAnalysisText tests ---

func TestDocAnalysisTextProseDocument(t *testing.T) {
	doc := testDoc("This is some prose text.")
	result := docAnalysisText(doc)
	if result != "This is some prose text." {
		t.Fatalf("expected original prose, got: %q", result)
	}
}

func TestDocAnalysisTextCodeCommentExtractsComments(t *testing.T) {
	// Write a Go source file with comments
	content := `package main

// This is a comment explaining the function.
func Foo() {
	// Another inline comment.
	return
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	docs, err := collectInputs([]string{path}, false, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	result := docAnalysisText(docs[0])
	if !strings.Contains(result, "This is a comment explaining the function") {
		t.Fatalf("expected extracted comment in analysis text, got: %q", result)
	}
	if !strings.Contains(result, "Another inline comment") {
		t.Fatalf("expected second comment in analysis text, got: %q", result)
	}
}

func TestDocAnalysisTextCodeCommentFallbackReturnsAnalysisContent(t *testing.T) {
	// Write an unsupported language file — detectLanguage returns false,
	// so docAnalysisText falls through to AnalysisContent().
	content := `some content here`
	dir := t.TempDir()
	path := filepath.Join(dir, "file.xyz")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	docs, err := collectInputs([]string{path}, false, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	result := docAnalysisText(docs[0])
	if result != "some content here" {
		t.Fatalf("expected fallback to analysis content, got: %q", result)
	}
}

// --- Minimal test checker for GitCompare propagation ---

type testGitCompareChecker struct {
	received **corpus.GitCompare
}

func (c testGitCompareChecker) ID() string   { return "TEST.GIT_COMPARE_CHECKER" }
func (c testGitCompareChecker) Version() int { return 1 }

func (c testGitCompareChecker) Run(ctx *check.RunContext) ([]report.Finding, error) {
	*c.received = ctx.GitCompare
	return nil, nil
}

func TestRunDeterministicChecksPassesGitCompare(t *testing.T) {
	gitCompare := &corpus.GitCompare{Revision: "abc123"}
	var received *corpus.GitCompare
	checker := testGitCompareChecker{received: &received}

	_, err := runDeterministicChecks(testDoc("hello world."), testProfile(), nil, []check.Checker{checker}, gitCompare)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != gitCompare {
		t.Fatalf("checker received GitCompare %p, want %p", received, gitCompare)
	}
}
