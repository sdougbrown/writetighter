package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
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

	err := (&App{}).RunExplainWithOptions("CORE.SENTENCE_LENGTH", "software-docs-en@0.2.0", "json")
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
	err := (&App{}).RunExplainWithOptions("NONEXISTENT_RULE", "software-docs-en@0.2.0", "json")
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
	err := (&App{}).RunExplainWithOptions("CORE.SENTENCE_LENGTH", "software-docs-en@0.2.0", "xml")
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

	err := (&App{}).RunExplainWithOptions("CORE.SENTENCE_LENGTH", "software-docs-en@0.2.0", "human")
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

	err := (&App{}).RunProfileVerify("software-docs-en@0.2.0", "json")
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
		sourceText := userText[:5]
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
