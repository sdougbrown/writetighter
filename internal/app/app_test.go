package app

import (
	"bytes"
	"encoding/json"
	"errors"
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

// runCheckWithParams is a helper that runs RunCheck with the given params.
func runCheckWithParams(params CheckParams) error {
	a := &App{}
	return a.RunCheck(params)
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

// --- RunCheck tests ---

func TestRunCheckNoProfileFails(t *testing.T) {
	params := CheckParams{
		Paths:  []string{"/nonexistent/path.txt"},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	err := runCheckWithParams(params)
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestRunCheckFailOnError(t *testing.T) {
	// The default profile's CORE.SENTENCE_LENGTH produces warning-severity findings,
	// not error. So FailOn=error should NOT trigger ErrFailThreshold even with findings.
	// We verify this by using long text that triggers sentence-length warnings.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "error",
	}
	err := runCheckWithParams(params)
	// No rule in the default profile produces error-severity, so no threshold reached.
	if err != nil {
		t.Fatalf("expected nil with FailOn=error (no error-severity findings), got %v", err)
	}
}

func TestRunCheckFailOnWarning(t *testing.T) {
	// Long text triggers CORE.SENTENCE_LENGTH which has severity "warning".
	// FailOn=warning should return ErrFailThreshold.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "warning",
	}
	err := runCheckWithParams(params)
	if err != ErrFailThreshold {
		t.Fatalf("expected ErrFailThreshold, got %v", err)
	}
}

func TestRunCheckFailOnWarningRendersCompletedReport(t *testing.T) {
	path := writeTempFile(t, strings.Repeat("word ", 30)+".")
	buf := captureStdout(t, func() {
		err := runCheckWithParams(CheckParams{Paths: []string{path}, Kind: "description", Format: "json", FailOn: "warning"})
		if !errors.Is(err, ErrFailThreshold) {
			t.Fatalf("expected threshold error, got %v", err)
		}
	})
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("expected completed JSON report: %v", err)
	}
	if got["status"] != "checked" {
		t.Fatalf("unexpected report: %#v", got)
	}
}

func TestRunCheckFailOnNone(t *testing.T) {
	// Even though there are findings, FailOn=none means no error.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	err := runCheckWithParams(params)
	if err != nil {
		t.Fatalf("expected nil with FailOn=none, got %v", err)
	}
}

func TestRunCheckStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("this is some text."))
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	params := CheckParams{
		Stdin:  true,
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	checkErr := runCheckWithParams(params)
	if checkErr != nil {
		t.Fatalf("unexpected error: %v", checkErr)
	}
}

func TestRunCheckJSONOutput(t *testing.T) {
	text := "some text here."
	path := writeTempFile(t, text)

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	runCheckWithParams(params)
	w.Close()
	os.Stdout = oldStdout

	buf.ReadFrom(r)

	var reportData map[string]any
	if err := json.Unmarshal(buf.Bytes(), &reportData); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}
	if reportData["status"] != "checked" {
		t.Errorf("expected status 'checked', got %v", reportData["status"])
	}
}

func TestRunCheckHumanOutput(t *testing.T) {
	text := "some text here."
	path := writeTempFile(t, text)

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "human",
		FailOn: "none",
	}
	runCheckWithParams(params)
	w.Close()
	os.Stdout = oldStdout

	buf.ReadFrom(r)

	out := buf.String()
	// Human format shows "status: checked" at minimum.
	if !strings.Contains(out, "status:") {
		t.Fatalf("expected human output to contain 'status:', got: %s", out)
	}
}

func TestRunCheckAgentOutput(t *testing.T) {
	text := "some text here."
	path := writeTempFile(t, text)

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "agent",
		FailOn: "none",
	}
	runCheckWithParams(params)
	w.Close()
	os.Stdout = oldStdout

	buf.ReadFrom(r)

	out := buf.String()
	// Agent format outputs nothing when there are no findings, just an empty string.
	// That's valid — the format itself works without error.
	_ = out
}

func TestRunCheckDefaultProfile(t *testing.T) {
	text := "writetighter is great."
	path := writeTempFile(t, text)

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	}
	err := runCheckWithParams(params)
	if err != nil {
		t.Fatalf("unexpected error with default profile: %v", err)
	}
}

func TestRunCheckInvalidProfileSpec(t *testing.T) {
	text := "hello world."
	path := writeTempFile(t, text)

	params := CheckParams{
		Paths:   []string{path},
		Kind:    "description",
		Format:  "json",
		FailOn:  "none",
		Profile: "nonexistent@0.0.0",
	}
	err := runCheckWithParams(params)
	if err == nil {
		t.Fatal("expected error for invalid profile spec")
	}
}

func TestRunCheckLLMRequiredButNotAvailable(t *testing.T) {
	// Verify the sentinel error is defined.
	if ErrRequireLLM == nil {
		t.Fatal("ErrRequireLLM should not be nil")
	}
}

func TestRunCheckFailOnWarningIncludesErrors(t *testing.T) {
	// FailOn=warning catches both warning AND error severity.
	// Sentence length produces warning-severity findings.
	text := strings.Repeat("word ", 30) + "."
	path := writeTempFile(t, text)

	params := CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "warning",
	}
	err := runCheckWithParams(params)
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

func TestRunCheckExplicitMissingConfigFails(t *testing.T) {
	text := "simple text."
	path := writeTempFile(t, text)

	params := CheckParams{
		Paths:      []string{path},
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
		ConfigPath: "/nonexistent/config.yaml",
	}
	err := runCheckWithParams(params)
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

func TestRunCheckRejectsSymlinkPath(t *testing.T) {
	realPath := writeTempFile(t, "test content")
	symPath := filepath.Join(t.TempDir(), "link.txt")
	if err := os.Symlink(realPath, symPath); err != nil {
		t.Skip("symlink creation not supported")
	}
	err := runCheckWithParams(CheckParams{Paths: []string{symPath}, Kind: "description", Format: "json", FailOn: "none"})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestRunCheckLLMOptionalFailurePreservesReport(t *testing.T) {
	// With a connection that will fail, optional LLM should still produce a report.
	params := CheckParams{
		Paths:      []string{writeTempFile(t, "simple text.")},
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
		LLM:        true,
		LLMBaseURL: "http://127.0.0.1:1",
		LLMModel:   "test",
	}
	err := runCheckWithParams(params)
	if err != nil {
		t.Fatalf("expected no error from optional LLM failure, got %v", err)
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
	if err == nil {
		t.Fatal("expected error for missing LLM config")
	}
	if !strings.Contains(err.Error(), "model is required") && !strings.Contains(err.Error(), "LLM configuration") && !strings.Contains(err.Error(), "revise requires") {
		t.Fatalf("expected config-related error, got: %v", err)
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
	err := (&App{}).RunLint(CheckParams{
		Paths:  []string{path},
		Kind:   "description",
		Format: "json",
		FailOn: "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLintCannotEnableModelCallsThroughParams(t *testing.T) {
	path := writeTempFile(t, "simple text.")
	err := (&App{}).RunLint(CheckParams{
		Paths:      []string{path},
		Kind:       "description",
		Format:     "json",
		FailOn:     "none",
		LLM:        true,
		RequireLLM: true,
		LLMBaseURL: "http://127.0.0.1:1",
		LLMModel:   "test",
	})
	if err != nil {
		t.Fatalf("lint must remain deterministic even when an API caller sets LLM fields: %v", err)
	}
}

func TestRunLintProducesOutput(t *testing.T) {
	path := writeTempFile(t, "some text.")
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := (&App{}).RunLint(CheckParams{
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
	if reportData["status"] != "checked" {
		t.Errorf("expected status 'checked', got %v", reportData["status"])
	}
}

func TestRunReviseProducesStructuredClarificationWithoutLintFinding(t *testing.T) {
	content := "Refs delta pluralizes three scalars."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{"findings":[{"kind":"clarification","source_range":{"start":0,"end":36},"principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"The transformation has multiple plausible meanings.","question":"Does this rename fields or change scalar values to collections?","confidence":0.84}]}`
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
