package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

func TestBuildRewritePromptContainsKindDirections(t *testing.T) {
	doc, err := document.FromText("Some text.", guidance.KindProcedure)
	if err != nil {
		t.Fatal(err)
	}
	sys, user := BuildRewritePrompt(doc, nil, nil, nil)
	if !strings.Contains(sys, "procedure") {
		t.Fatal("expected kind in system prompt")
	}
	if !strings.Contains(sys, "prerequisites") {
		t.Fatal("expected procedure-specific direction")
	}
	if !strings.Contains(user, "<rewrite-text>") {
		t.Fatal("expected <rewrite-text> wrapper in user content")
	}
	if !strings.Contains(user, "Some text.") {
		t.Fatal("expected source text in user content")
	}
}

func TestBuildRewritePromptDoesNotAskForClarifications(t *testing.T) {
	doc, err := document.FromText("Some text.", guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	sys, _ := BuildRewritePrompt(doc, nil, nil, nil)
	if !strings.Contains(sys, "preserve it as-is") {
		t.Fatal("expected best-effort 'preserve as-is' directive")
	}
	if strings.Contains(sys, "ask a concrete clarification question") {
		t.Fatal("rewrite prompt should not ask for clarifications")
	}
}

func TestBuildRewritePromptIncludesLintFindings(t *testing.T) {
	doc, err := document.FromText("Some text here.", guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	findings := []report.Finding{
		{RuleID: "CORE.SENTENCE_LENGTH", Message: "sentence too long", Range: &report.FindingRange{StartByte: 0, EndByte: 10}},
	}
	sys, _ := BuildRewritePrompt(doc, nil, findings, nil)
	if !strings.Contains(sys, "lint findings for context") {
		t.Fatal("expected lint findings in system prompt")
	}
	if !strings.Contains(sys, "CORE.SENTENCE_LENGTH") {
		t.Fatal("expected rule ID in system prompt")
	}
}

func TestBuildRewritePromptIncludesGlossaryTerms(t *testing.T) {
	doc, err := document.FromText("Deploy the WidgetProcessor.", guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	terms := []config.TermEntry{
		{Term: "WidgetProcessor", Definition: "A service that processes widgets."},
	}
	sys, _ := BuildRewritePrompt(doc, nil, nil, terms)
	if !strings.Contains(sys, "project glossary") {
		t.Fatal("expected glossary section")
	}
	if !strings.Contains(sys, "WidgetProcessor") {
		t.Fatal("expected glossary term in system prompt")
	}
}

func TestRewriteReturnsRewrittenText(t *testing.T) {
	original := "The component leverages synergies."
	rewritten := "The component uses synergies."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no response_format is sent.
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if _, ok := req["response_format"]; ok {
			t.Errorf("rewrite should not send response_format")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": rewritten}}},
		})
	}))
	defer server.Close()

	doc, err := document.FromText(original, guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{BaseURL: server.URL, Model: "test-model"}
	result, err := Rewrite(context.Background(), cfg, doc, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != rewritten {
		t.Fatalf("expected %q, got %q", rewritten, result.Text)
	}
	if result.Discarded {
		t.Fatal("expected discarded=false")
	}
}

func TestRewriteDiscardsOnProtectedContentLoss(t *testing.T) {
	original := "Deploy the API service using version v2.3.1."
	unsafeRewrite := "Deploy the service."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": unsafeRewrite}}},
		})
	}))
	defer server.Close()

	doc, err := document.FromText(original, guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{BaseURL: server.URL, Model: "test-model"}
	result, err := Rewrite(context.Background(), cfg, doc, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Discarded {
		t.Fatal("expected discarded=true")
	}
	if result.Text != original {
		t.Fatalf("expected original returned, got %q", result.Text)
	}
}

func TestRewriteEmptyResponseReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": ""}}},
		})
	}))
	defer server.Close()

	doc, err := document.FromText("Some text.", guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{BaseURL: server.URL, Model: "test-model"}
	_, err = Rewrite(context.Background(), cfg, doc, nil, nil, nil)
	if err != ErrRewriteEmpty {
		t.Fatalf("expected ErrRewriteEmpty, got %v", err)
	}
}

func TestRewriteStripsMarkdownFence(t *testing.T) {
	rewritten := "The module handles requests."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fenced := "```\n" + rewritten + "\n```"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": fenced}}},
		})
	}))
	defer server.Close()

	doc, err := document.FromText("The module processes incoming requests.", guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{BaseURL: server.URL, Model: "test-model"}
	result, err := Rewrite(context.Background(), cfg, doc, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != rewritten {
		t.Fatalf("expected %q, got %q", rewritten, result.Text)
	}
}

func TestRewritePreservesSafeRewriteWithProtectedTokens(t *testing.T) {
	// The rewrite keeps all protected tokens (API, v2.3.1) even though it
	// restructures the sentence.
	original := "Deploy the API service using version v2.3.1 of the container."
	rewritten := "Deploy version v2.3.1 of the API service container."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": rewritten}}},
		})
	}))
	defer server.Close()

	doc, err := document.FromText(original, guidance.KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{BaseURL: server.URL, Model: "test-model"}
	result, err := Rewrite(context.Background(), cfg, doc, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Discarded {
		t.Fatal("expected discarded=false (protected tokens preserved)")
	}
	if result.Text != rewritten {
		t.Fatalf("expected %q, got %q", rewritten, result.Text)
	}
}

// silence unused imports
var _ = profile.Resolve
