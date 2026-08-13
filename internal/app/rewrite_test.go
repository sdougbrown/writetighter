package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/report"
)

func TestRunRewriteNoInput(t *testing.T) {
	err := (&App{}).RunRewrite(RewriteParams{})
	if err == nil {
		t.Fatal("expected error for no input")
	}
	if !strings.Contains(err.Error(), "no input specified") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestRunRewriteMutuallyExclusive(t *testing.T) {
	err := (&App{}).RunRewrite(RewriteParams{
		Stdin: true,
		Paths: []string{"file.md"},
	})
	if err == nil {
		t.Fatal("expected error for stdin+paths")
	}
}

func TestRunRewriteInvalidKind(t *testing.T) {
	err := (&App{}).RunRewrite(RewriteParams{
		Text: strPtr("Short text."),
		Kind: "invalid-kind",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid document kind") {
		t.Fatalf("expected invalid kind error, got %v", err)
	}
}

func TestRunRewriteInvalidFormat(t *testing.T) {
	err := (&App{}).RunRewrite(RewriteParams{
		Text:   strPtr("Short text."),
		Format: "xml",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid format") {
		t.Fatalf("expected invalid format error, got %v", err)
	}
}

func TestRunRewriteMissingLLMConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	path := writeTempFile(t, "short test.")
	err := (&App{}).RunRewrite(RewriteParams{
		Paths: []string{path},
	})
	if !errors.Is(err, ErrLLMConfigRequired) {
		t.Fatalf("expected ErrLLMConfigRequired, got %v", err)
	}
}

func TestRunRewriteTextOutput(t *testing.T) {
	original := "The component leverages synergies to facilitate the onboarding process for new users."
	rewritten := "The component helps new users get started."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": rewritten}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	buf := captureStdout(t, func() {
		text := original
		err := (&App{}).RunRewrite(RewriteParams{
			Text:   &text,
			Kind:   "description",
			Format: "text",
		})
		if err != nil {
			t.Fatalf("unexpected rewrite error: %v", err)
		}
	})

	got := strings.TrimSpace(buf.String())
	if got != rewritten {
		t.Fatalf("expected rewritten text %q, got %q", rewritten, got)
	}
}

func TestRunRewriteJSONOutput(t *testing.T) {
	original := "The system utilizes a configuration mechanism to initialize parameters."
	rewritten := "The system uses a configuration file to set parameters."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no response_format is sent (rewrite uses plain text, not structured JSON).
		var req struct {
			ResponseFormat *json.RawMessage `json:"response_format"`
			Messages       []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat != nil {
			t.Errorf("rewrite request should not include response_format")
		}
		// Verify the user content is wrapped in <rewrite-text> tags.
		userMsg := req.Messages[len(req.Messages)-1].Content
		if !strings.Contains(userMsg, "<rewrite-text>") {
			t.Errorf("expected <rewrite-text> wrapper in user content")
		}
		// Verify the system prompt contains rewrite-specific language.
		sysMsg := req.Messages[0].Content
		if !strings.Contains(sysMsg, "rewriter") {
			t.Errorf("expected 'rewriter' in system prompt")
		}
		if !strings.Contains(sysMsg, "preserve it as-is") {
			t.Errorf("expected best-effort 'preserve as-is' directive in system prompt")
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": rewritten}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	text := original
	buf := captureStdout(t, func() {
		err := (&App{}).RunRewrite(RewriteParams{
			Text:   &text,
			Kind:   "description",
			Format: "json",
		})
		if err != nil {
			t.Fatalf("unexpected rewrite error: %v", err)
		}
	})

	var resp report.RewriteResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid rewrite JSON: %v\n%s", err, buf.String())
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q", resp.Status)
	}
	if resp.RewrittenText != rewritten {
		t.Fatalf("expected rewritten text %q, got %q", rewritten, resp.RewrittenText)
	}
	if resp.Discarded {
		t.Fatal("expected discarded=false")
	}
	if resp.LLMModel != "test-model" {
		t.Fatalf("expected model test-model, got %q", resp.LLMModel)
	}
}

func TestRunRewriteDiscardsUnsafeRewrite(t *testing.T) {
	// The model drops protected tokens (API and v2.3.1) from the rewrite.
	// The hook should detect this and return the original text.
	original := "Deploy the API service using version v2.3.1 of the container image."
	unsafeRewrite := "Deploy the service using the container image."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": unsafeRewrite}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	text := original
	buf := captureStdout(t, func() {
		err := (&App{}).RunRewrite(RewriteParams{
			Text:   &text,
			Kind:   "procedure",
			Format: "json",
		})
		if err != nil {
			t.Fatalf("unexpected rewrite error: %v", err)
		}
	})

	var resp report.RewriteResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid rewrite JSON: %v\n%s", err, buf.String())
	}
	if !resp.Discarded {
		t.Fatal("expected discarded=true (protected content lost)")
	}
	if resp.RewrittenText != original {
		t.Fatalf("expected original text returned, got %q", resp.RewrittenText)
	}
	if resp.Status != "discarded" {
		t.Fatalf("expected status discarded, got %q", resp.Status)
	}
}

func TestRunRewriteEmptyResponseReturnsOriginal(t *testing.T) {
	original := "The configuration file controls the service behavior."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": ""}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	text := original
	buf := captureStdout(t, func() {
		err := (&App{}).RunRewrite(RewriteParams{
			Text:   &text,
			Kind:   "description",
			Format: "text",
		})
		if err != nil {
			t.Fatalf("unexpected rewrite error: %v", err)
		}
	})

	got := strings.TrimSpace(buf.String())
	if got != original {
		t.Fatalf("expected original text on empty response, got %q", got)
	}
}

func TestRunRewriteModelFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("model unavailable"))
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	text := "Some text here."
	err := (&App{}).RunRewrite(RewriteParams{
		Text:   &text,
		Kind:   "description",
		Format: "text",
	})
	if !errors.Is(err, ErrRewriteFailed) {
		t.Fatalf("expected ErrRewriteFailed, got %v", err)
	}
}

func TestRunRewriteStripsMarkdownFence(t *testing.T) {
	original := "The module processes incoming requests and returns responses."
	rewritten := "The module handles requests and sends responses."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Model wraps output in a Markdown fence despite instructions.
		fenced := "```json\n" + rewritten + "\n```"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": fenced}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	text := original
	buf := captureStdout(t, func() {
		err := (&App{}).RunRewrite(RewriteParams{
			Text:   &text,
			Kind:   "description",
			Format: "text",
		})
		if err != nil {
			t.Fatalf("unexpected rewrite error: %v", err)
		}
	})

	got := strings.TrimSpace(buf.String())
	if got != rewritten {
		t.Fatalf("expected fence-stripped text %q, got %q", rewritten, got)
	}
}

func TestRunRewriteStdin(t *testing.T) {
	original := "The API endpoint accepts JSON payloads and returns structured responses."
	rewritten := "The API endpoint takes JSON and returns structured responses."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": rewritten}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	// Capture stdin.
	oldStdin := os.Stdin
	rPipe, wPipe, _ := os.Pipe()
	os.Stdin = rPipe
	_, _ = wPipe.Write([]byte(original))
	wPipe.Close()
	defer func() { os.Stdin = oldStdin }()

	buf := captureStdout(t, func() {
		err := (&App{}).RunRewrite(RewriteParams{
			Stdin:  true,
			Kind:   "description",
			Format: "text",
		})
		if err != nil {
			t.Fatalf("unexpected rewrite error: %v", err)
		}
	})

	got := strings.TrimSpace(buf.String())
	if got != rewritten {
		t.Fatalf("expected %q, got %q", rewritten, got)
	}
}

func TestRunRewritePassesLintFindingsAsContext(t *testing.T) {
	// A very long sentence that triggers CORE.SENTENCE_LENGTH.
	longSentence := "This is an excessively long sentence that goes on and on about the various intricacies of the system configuration mechanism and how it interacts with multiple subsystems to produce a result that is ultimately beneficial to the end user experience."
	rewritten := "Short rewritten text."

	var systemPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		systemPrompt = req.Messages[0].Content

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": rewritten}}},
		})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")

	text := longSentence
	buf := captureStdout(t, func() {
		err := (&App{}).RunRewrite(RewriteParams{
			Text:   &text,
			Kind:   "description",
			Format: "json",
		})
		if err != nil {
			t.Fatalf("unexpected rewrite error: %v", err)
		}
	})

	var resp report.RewriteResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid rewrite JSON: %v\n%s", err, buf.String())
	}
	if resp.LintFindings == 0 {
		t.Fatal("expected lint findings to be passed as context")
	}
	if !strings.Contains(systemPrompt, "lint findings for context") {
		t.Fatal("expected lint findings in system prompt")
	}
}

// strPtr returns a pointer to s. Used for *string params.
func strPtr(s string) *string { return &s }
