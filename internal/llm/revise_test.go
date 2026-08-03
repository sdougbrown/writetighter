package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/reference"
	"github.com/sdougbrown/writetighter/internal/report"
)

func TestReviseResponseModes(t *testing.T) {
	if got, err := buildReviseResponseFormat("auto"); got != nil || err != nil {
		t.Fatalf("auto must rely on prompt JSON, got %#v, err %v", got, err)
	}
	if got, err := buildReviseResponseFormat("prompt_json"); got != nil || err != nil {
		t.Fatalf("prompt_json must rely on prompt JSON, got %#v, err %v", got, err)
	}
	got, err := buildReviseResponseFormat("json_object")
	if err != nil || got == nil || got.Type != "json_object" {
		t.Fatalf("json_object revise format = %#v, err %v", got, err)
	}
	if got.JSONSchema != nil {
		t.Fatalf("json_object must not set JSONSchema, got %#v", got.JSONSchema)
	}
	got, err = buildReviseResponseFormat("json_schema")
	if err != nil || got == nil || got.JSONSchema == nil {
		t.Fatalf("missing revise JSON schema: %#v, err %v", got, err)
	}
	schema := string(got.JSONSchema.Schema)
	for _, expected := range []string{`"source_text"`, `"principle_ids"`, `"clarification"`, `"replacement"`, `"question"`, `"CORE.EXPLICIT_RELATIONSHIPS"`, `"CORE.CAUSAL_ORDER"`, `"CORE.PLAIN_MECHANISM"`, `"CORE.RELEVANT_DETAIL"`} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("revise schema missing %s: %s", expected, schema)
		}
	}
	if strings.Contains(schema, "uniqueItems") || strings.Contains(schema, `"rule_ids"`) {
		t.Fatalf("revise schema contains unsupported or unexpected fields: %s", schema)
	}
}

func TestValidateReviseResponseValid(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"use preferred term","replacement":"preferred term","confidence":0.91}]}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("expected valid response, got: %v", err)
	}
	if len(resp.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(resp.Revisions))
	}
	if resp.Revisions[0].Kind != "rewrite" {
		t.Fatalf("expected rewrite, got %q", resp.Revisions[0].Kind)
	}
}

func TestValidateReviseResponseClarification(t *testing.T) {
	raw := `{"findings":[{"kind":"clarification","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.APPROVED_WORDS"],"reason":"unclear term","question":"Did you mean X?","confidence":0.72}]}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("expected valid clarification, got: %v", err)
	}
	if resp.Revisions[0].Kind != "clarification" {
		t.Fatalf("expected clarification, got %q", resp.Revisions[0].Kind)
	}
	if resp.Revisions[0].Replacement != nil {
		t.Fatal("clarification must not have replacement")
	}
	if resp.Revisions[0].Question == nil {
		t.Fatal("clarification must have question")
	}
}

func TestValidateReviseResponseRequiresSourceText(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"test","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "source_text") {
		t.Fatalf("expected source_text error, got %v", err)
	}
}

func TestValidateReviseResponseInvalidKind(t *testing.T) {
	raw := `{"findings":[{"kind":"delete","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"test","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "invalid revision kind") {
		t.Fatalf("expected invalid kind error, got: %v", err)
	}
}

func TestValidateReviseResponseRewriteWithoutReplacement(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"test","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "requires replacement") {
		t.Fatalf("expected requires replacement error, got: %v", err)
	}
}

func TestValidateReviseResponseClarificationWithoutQuestion(t *testing.T) {
	raw := `{"findings":[{"kind":"clarification","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.APPROVED_WORDS"],"reason":"test","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "must not have replacement") {
		t.Fatalf("expected no replacement for clarification, got: %v", err)
	}
}

func TestValidateReviseResponseMissingPrincipleIDs(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":[],"reason":"test","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "missing principle_ids") {
		t.Fatalf("expected missing principle_ids error, got: %v", err)
	}
}

func TestValidateReviseResponseUnknownPrinciple(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["UNKNOWN.PRINCIPLE"],"reason":"test","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "unknown principle id") {
		t.Fatalf("expected unknown principle error, got: %v", err)
	}
}

func TestValidateReviseResponseInvalidConfidence(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"test","replacement":"x","confidence":1.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "invalid revise confidence") {
		t.Fatalf("expected invalid confidence error, got: %v", err)
	}
}

func TestValidateReviseResponseClaimLanguage(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"this is certified","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "changes claims") {
		t.Fatalf("expected claim rejection, got: %v", err)
	}
}

func TestValidateReviseResponseTooLarge(t *testing.T) {
	// Create a response that exceeds MaxOutputChars bytes.
	raw := make([]byte, MaxOutputChars+100)
	for i := range raw {
		raw[i] = 'x'
	}
	_, err := ValidateReviseResponse(raw)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too large error, got: %v", err)
	}
}

func TestValidateReviseResponseEmptyReason(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"   ","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "empty reason") {
		t.Fatalf("expected empty reason error, got: %v", err)
	}
}

func TestValidateReviseResponseEmptyReplacement(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"test","replacement":"   ","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "empty replacement") {
		t.Fatalf("expected empty replacement error, got: %v", err)
	}
}

func TestValidateReviseResponseEmptyQuestion(t *testing.T) {
	raw := `{"findings":[{"kind":"clarification","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.APPROVED_WORDS"],"reason":"test","question":"   ","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "empty question") {
		t.Fatalf("expected empty question error, got: %v", err)
	}
}

func TestValidateReviseResponseDuplicatePrinciple(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE","CORE.SHORT_SENTENCE"],"reason":"test","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "duplicate principle id") {
		t.Fatalf("expected duplicate principle error, got: %v", err)
	}
}

func TestValidateReviseResponseAllowsOverlappingAdvisoryRanges(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":10},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"first","replacement":"a","confidence":0.5},{"kind":"clarification","source_text":"hello","source_range":{"start":5,"end":15},"principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"second","question":"What changes?","confidence":0.5}]}`
	response, err := ValidateReviseResponse([]byte(raw))
	if err != nil || len(response.Revisions) != 2 {
		t.Fatalf("expected overlapping advisory alternatives to pass, got %#v, err=%v", response, err)
	}
}

func TestValidateReviseResponseAllowsRepeatedReasonForSeparateRanges(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":10,"end":15},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"same reason","replacement":"a","confidence":0.5},{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.APPROVED_WORDS"],"reason":"same reason","replacement":"b","confidence":0.5}]}`
	response, err := ValidateReviseResponse([]byte(raw))
	if err != nil || len(response.Revisions) != 2 {
		t.Fatalf("expected repeated reason on separate unordered ranges to pass, got %#v, err=%v", response, err)
	}
}

func TestValidateReviseResponseRejectsEmptyRange(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":5,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"test","replacement":"x","confidence":0.5}]}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "invalid revision source range") {
		t.Fatalf("expected empty range error, got %v", err)
	}
}

func TestValidateReviseResponseEmpty(t *testing.T) {
	raw := `{"findings":[]}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("expected valid empty response, got: %v", err)
	}
	if len(resp.Revisions) != 0 {
		t.Fatalf("expected 0 revisions, got %d", len(resp.Revisions))
	}
}

func TestBuildRevisePromptBasic(t *testing.T) {
	content := "This is a test sentence for revision."
	doc, _ := document.FromReader(strings.NewReader(content), "test.md", "description")
	res := &profile.Resolution{ID: "PROFILE_ID", Version: "1", Dict: nil}
	prompt, excerpt := BuildRevisePrompt(doc, res, nil, nil)

	// Prompt must include revise-specific instructions.
	if !strings.Contains(prompt, "Revision principles") {
		t.Fatal("expected revision principles in prompt")
	}
	if !strings.Contains(prompt, "subject, action, object, and effect explicit") {
		t.Fatal("expected rubric about making subject/action/object explicit")
	}
	if !strings.Contains(prompt, "active, direct voice") {
		t.Fatal("expected rubric about active/direct voice")
	}
	if !strings.Contains(prompt, "short sentences") {
		t.Fatal("expected rubric about short sentences")
	}
	if !strings.Contains(prompt, "one topic per paragraph") {
		t.Fatal("expected rubric about one topic per paragraph")
	}
	if !strings.Contains(prompt, "one term per concept") {
		t.Fatal("expected rubric about one term per concept")
	}
	if !strings.Contains(prompt, "merely polish grammar") || !strings.Contains(prompt, "clarification question") {
		t.Fatal("expected semantic-clarity gate before rewriting")
	}
	for _, direction := range []string{"CORE.CAUSAL_ORDER", "CORE.PLAIN_MECHANISM", "CORE.RELEVANT_DETAIL", "causal sequence", "Do not add a rationale"} {
		if !strings.Contains(prompt, direction) {
			t.Fatalf("expected explanation direction %q", direction)
		}
	}
	// Prompt should include the fixed principle IDs in the JSON example.
	if !strings.Contains(prompt, "CORE.SHORT_SENTENCE") {
		t.Fatal("expected prompt to include principle ID in JSON example")
	}

	// Excerpt should contain the full document content.
	if !strings.Contains(excerpt.Text, "test sentence") {
		t.Fatal("expected test sentence in excerpt")
	}
}

func TestBuildRevisePromptAddsPRDecisionOrder(t *testing.T) {
	doc, _ := document.FromReader(strings.NewReader("This PR changes the loader."), "test.md", "pr")
	prompt, _ := BuildRevisePrompt(doc, &profile.Resolution{ID: "PROFILE_ID", Version: "1"}, nil, nil)
	if !strings.Contains(prompt, "Prioritize what changes") || !strings.Contains(prompt, "context or requirement, its implication, then the implementation choice") {
		t.Fatalf("missing PR-specific explanation guidance: %s", prompt)
	}
}

func TestBuildRevisePromptUsesStatusUpdateExample(t *testing.T) {
	doc, _ := document.FromReader(strings.NewReader("The work is still running."), "test.md", guidance.KindStatusUpdate)
	prompt, _ := BuildRevisePrompt(doc, &profile.Resolution{ID: "PROFILE_ID", Version: "1"}, nil, nil)
	for _, expected := range []string{"Primary document-kind objective (status-update)", "observable progress", "current hypothesis", "diagnostic action happens next"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("status prompt missing %q: %s", expected, prompt)
		}
	}
	if strings.Index(prompt, "Primary document-kind objective") > strings.Index(prompt, "Revision principles") {
		t.Fatal("document-kind objective must precede shared principles")
	}
}

func TestBuildRevisePromptUsesExportedGuidanceForEveryKind(t *testing.T) {
	for _, kind := range guidance.Kinds() {
		doc, _ := document.FromReader(strings.NewReader("Technical prose."), "test.md", kind)
		prompt, _ := BuildRevisePrompt(doc, &profile.Resolution{ID: "PROFILE_ID", Version: "1"}, nil, nil)
		rubric, err := guidance.ForKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		for _, principle := range rubric.Principles {
			if !strings.Contains(prompt, principle.ID) || !strings.Contains(prompt, principle.Direction) {
				t.Fatalf("%s prompt omitted principle %#v", kind, principle)
			}
		}
		for _, direction := range append(append([]string{}, rubric.CoreDirections...), rubric.KindDirections...) {
			if !strings.Contains(prompt, direction) {
				t.Fatalf("%s prompt omitted direction %q", kind, direction)
			}
		}
	}
}

func TestBuildReviseWritingGuidelinesUsesOnlyRelevantReviewedPolicy(t *testing.T) {
	d := &profile.Dictionary{FormatVersion: 1, Entries: []profile.Entry{
		{Term: "utilize", Status: profile.StatusDiscouraged, Alternatives: []string{"use"}, Reason: "Use the direct term.", PartsOfSpeech: []string{"verb"}},
		{Term: "use", Status: profile.StatusPreferred, PartsOfSpeech: []string{"verb"}},
		{Term: "irrelevant phrase", Status: profile.StatusDiscouraged, Reason: "Recast it.", PartsOfSpeech: []string{"phrase"}},
		{Term: "fixture", Status: profile.StatusObserved},
	}}
	_ = d.Validate()
	got := BuildReviseWritingGuidelines(d, "Utilize the existing fixture.")
	if !strings.Contains(got, "utilize") || !strings.Contains(got, "use") {
		t.Fatalf("expected relevant reviewed mapping, got %q", got)
	}
	if strings.Contains(got, "irrelevant phrase") || strings.Contains(got, "fixture") {
		t.Fatalf("included unrelated or observed vocabulary: %q", got)
	}
	if got := BuildReviseWritingGuidelines(d, "Use the existing fixture."); got != "" {
		t.Fatalf("preferred alternative alone should not pull unrelated discouraged policy into the prompt: %q", got)
	}
}

func TestBuildReviseWritingGuidelinesMarksGuidanceOnlyTerms(t *testing.T) {
	d := &profile.Dictionary{FormatVersion: 1, Entries: []profile.Entry{
		{Term: "when it comes to", Status: profile.StatusDiscouraged, Reason: "State the subject directly.", PartsOfSpeech: []string{"phrase"}},
	}}
	got := BuildReviseWritingGuidelines(d, "When it comes to retries, set a limit.")
	if !strings.Contains(got, "no fixed replacement") || !strings.Contains(got, "do not delete") {
		t.Fatalf("expected contextual recast guidance, got %q", got)
	}
}

func TestBuildRevisePromptWithTerms(t *testing.T) {
	content := "Use the project term correctly."
	doc, _ := document.FromReader(strings.NewReader(content), "test.md", "description")
	res := &profile.Resolution{ID: "PROFILE_ID", Version: "1", Dict: nil}
	terms := []config.TermEntry{{Term: "project term", Definition: "A defined term."}}
	prompt, _ := BuildRevisePrompt(doc, res, nil, terms)
	if !strings.Contains(prompt, "project glossary") {
		t.Fatal("expected project glossary in prompt")
	}
	if !strings.Contains(prompt, "project term") {
		t.Fatal("expected project term in prompt")
	}
}

func TestBuildRevisePromptWithFindings(t *testing.T) {
	content := "This sentence has the deprecated term in it."
	doc, _ := document.FromReader(strings.NewReader(content), "test.md", "description")
	res := &profile.Resolution{
		ID: "PROFILE_ID", Version: "1",
		Rules: &profile.RulesConfig{
			Rules: []profile.Rule{
				{ID: "CORE.TERM_DISCOURAGED", Enabled: true},
			},
		},
	}
	findings := []report.Finding{{
		RuleID:  "CORE.TERM_DISCOURAGED",
		Message: "use preferred term",
		Range:   &report.FindingRange{StartByte: 18, EndByte: 33},
	}}
	prompt, _ := BuildRevisePrompt(doc, res, findings, nil)
	// Should include lint findings as context.
	if !strings.Contains(prompt, "lint findings for context") {
		t.Fatal("expected lint findings section in prompt")
	}
	if !strings.Contains(prompt, "CORE.TERM_DISCOURAGED") {
		t.Fatal("expected rule ID in prompt")
	}
	// Prompt should not say "prerequisites" or "required".
	if strings.Contains(prompt, "prerequisite") {
		t.Fatal("lint findings should be context, not prerequisites")
	}
	// Should clarify that lint rules are not revision principles.
	if !strings.Contains(prompt, "deterministic checks, not revision principles") {
		t.Fatal("expected prompt to clarify lint findings are not revision principles")
	}
}

func TestBuildRevisePromptWithoutFindings(t *testing.T) {
	content := "Short text without issues."
	doc, _ := document.FromReader(strings.NewReader(content), "test.md", "description")
	res := &profile.Resolution{ID: "PROFILE_ID", Version: "1", Dict: nil}
	prompt, excerpt := BuildRevisePrompt(doc, res, nil, nil)
	// Should still include rubric even without findings.
	if !strings.Contains(prompt, "Revision principles") {
		t.Fatal("expected revision principles even without findings")
	}
	// Excerpt should still contain the full content.
	if !strings.Contains(excerpt.Text, "Short text") {
		t.Fatal("expected content in excerpt even without findings")
	}
}

func TestReviseWithFakeServer(t *testing.T) {
	srv := newFakeReviseServer(false, "ok")
	defer srv.Close()

	content := "deprecated term appears here."
	doc, _ := document.FromReader(strings.NewReader(content), "test.md", "description")
	res := &profile.Resolution{
		ID: "PROFILE_ID", Version: "1",
		Rules: &profile.RulesConfig{
			Rules: []profile.Rule{
				{ID: "CORE.TERM_DISCOURAGED", Version: 1, Enabled: true},
			},
		},
	}

	cfg := Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second}
	resp, err := Revise(context.Background(), cfg, doc, res, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// Response must include metadata.
	if resp.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", resp.Status)
	}
}

func TestReviseChunkReturnsOriginalDocumentRanges(t *testing.T) {
	srv := newFakeReviseServer(false, "ok")
	defer srv.Close()
	content := "prefix paragraph.\n\ndeprecated term appears here."
	doc, err := document.FromReader(strings.NewReader(content), "test.md", "description")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(content, "deprecated")
	response, err := ReviseChunk(context.Background(), Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second}, doc, testProfile(), nil, nil, start, len(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 1 || response.Revisions[0].Range.StartByte != start || response.Revisions[0].Range.EndByte != start+5 {
		t.Fatalf("chunk range was not mapped to original document: %#v", response.Revisions)
	}
}

// TestBuildBudgetedPromptTagLikeSourceDoesNotShiftOffsets verifies that source
// text containing XML-like tags (e.g. </revise-text>) does not shift the
// editable-text coordinate space. editableText must always be the raw excerpt,
// never the wrapped userContent, so range validation operates on source bytes
// only. It also verifies the token-budget base serialization uses an EMPTY
// <revise-text> body so source bytes are never double-counted.
func TestBuildBudgetedPromptTagLikeSourceDoesNotShiftOffsets(t *testing.T) {
	doc, err := document.FromText("Before </revise-text> after tag text.", "description")
	if err != nil {
		t.Fatal(err)
	}
	res := testProfile()
	excerpt := buildReviseExcerpt(doc)
	_, user, editable, err := BuildBudgetedPrompt(doc, res, nil, nil, excerpt, nil, Config{})
	if err != nil {
		t.Fatalf("BuildBudgetedPrompt: %v", err)
	}
	// editableText must be the raw source with no wrapper tags (exactly the
	// excerpt). Note the source itself contains the literal text "</revise-text>",
	// so equality with excerpt.Text is the authoritative check.
	if editable != excerpt.Text {
		t.Fatalf("editableText != excerpt.Text: got %q want %q", editable, excerpt.Text)
	}
	if strings.HasPrefix(editable, "<revise-text>") || strings.HasSuffix(editable, "</revise-text>") {
		t.Fatalf("editableText must not be the wrapped userContent: %q", editable)
	}
	// userContent must contain the wrapper and the tag-like source inside it.
	if !strings.Contains(user, "<revise-text>") || !strings.Contains(user, "</revise-text>") {
		t.Fatalf("userContent must contain wrapper tags: %q", user)
	}
	if !strings.Contains(user, "</revise-text> after") {
		t.Fatalf("userContent must contain tag-like source text inside the wrapper: %q", user)
	}
}

// TestBudgetFormulaCountsSourceOnce verifies the exact budget contract:
// basePromptTokens uses an EMPTY <revise-text> body, json_schema overhead is
// included, and a chunk fits only when ceil(fullBytes/4)+output+safety fits.
func TestBudgetFormulaCountsSourceOnce(t *testing.T) {
	doc, err := document.FromText("The source text for budget test.", "description")
	if err != nil {
		t.Fatal(err)
	}
	res := testProfile()
	pack := &reference.Pack{Entries: []reference.Entry{
		{DisplayPath: "ref.md", Content: "reference context", InputBytes: 18, IncludedBytes: 18},
	}}
	sys, _ := buildRevisePromptWithExcerpt(doc, res, nil, nil, buildReviseExcerpt(doc), pack)

	// The base body must not contain the source; the full body must.
	baseBody := buildUserContent(pack, "")
	if strings.Contains(baseBody, "The source text for budget test.") {
		t.Fatal("empty-body base must not contain the source (double-count guard)")
	}
	fullBody := buildUserContent(pack, "The source text for budget test.")
	if !strings.Contains(fullBody, "The source text for budget test.") {
		t.Fatal("full body must contain the source")
	}

	cfg := Config{Model: "m", ResponseMode: "json_object", ContextWindowTokens: 4096, MaxOutputTokens: 1024}
	baseBytes, err := SerializeRequestBytes(cfg, sys, baseBody)
	if err != nil {
		t.Fatal(err)
	}
	fullBytes, err := SerializeRequestBytes(cfg, sys, fullBody)
	if err != nil {
		t.Fatal(err)
	}
	if fullBytes <= baseBytes {
		t.Fatalf("fullBytes=%d should exceed baseBytes=%d", fullBytes, baseBytes)
	}
	baseTokens := (baseBytes + 3) / 4
	if available := cfg.ContextWindowTokens - baseTokens - cfg.MaxOutputTokens - config.BudgetSafetyTokens; available < config.MinEditableSourceTokens {
		t.Fatalf("available source budget %d below min %d", available, config.MinEditableSourceTokens)
	}

	// json_schema must be budgeted (its schema bytes are part of the serialized
	// request).
	schemaCfg := cfg
	schemaCfg.ResponseMode = "json_schema"
	schemaBytes, err := SerializeRequestBytes(schemaCfg, sys, baseBody)
	if err != nil {
		t.Fatal(err)
	}
	if schemaBytes <= baseBytes {
		t.Fatalf("json_schema serialization (%d) must exceed json_object (%d) by schema bytes", schemaBytes, baseBytes)
	}

	// An over-budget chunk must be rejected: build an excerpt large enough that
	// the base leaves >= minEditableSourceTokens but the full serialization does
	// not fit, and assert the candidate-fit error fires.
	source := strings.Repeat("budgetable source text words. ", 100) // ~3.2 KiB, well above minEditableSourceTokens
	bigDoc, err := document.FromText(source, "description")
	if err != nil {
		t.Fatal(err)
	}
	bigExcerpt := buildReviseExcerpt(bigDoc)
	overCfg := Config{Model: "m", ResponseMode: "json_object",
		// Leave room for ~512 source tokens: base must pass, but the ~800-token
		// excerpt cannot fit.
		ContextWindowTokens: baseTokens + cfg.MaxOutputTokens + config.BudgetSafetyTokens + 512,
		MaxOutputTokens:     cfg.MaxOutputTokens,
	}
	_, _, _, err = BuildBudgetedPrompt(bigDoc, res, nil, nil, bigExcerpt, pack, overCfg)
	if err == nil || !strings.Contains(err.Error(), "context_window_tokens") {
		t.Fatalf("expected candidate-fit rejection, got: %v", err)
	}
	// If the window cannot even leave minEditableSourceTokens for the base, the
	// actionable configuration error must fire.
	tinyCfg := Config{Model: "m", ResponseMode: "json_object",
		ContextWindowTokens: baseTokens + cfg.MaxOutputTokens + config.BudgetSafetyTokens - 10,
		MaxOutputTokens:     cfg.MaxOutputTokens,
	}
	_, _, _, err = BuildBudgetedPrompt(bigDoc, res, nil, nil, bigExcerpt, pack, tinyCfg)
	if err == nil || !strings.Contains(err.Error(), "revision context requires") {
		t.Fatalf("expected actionable configuration error, got: %v", err)
	}
}

// TestReviseChunkTagLikeSourceRangeValidation does an end-to-end check that a
// model finding whose source range references text containing a closing
// </revise-text> tag validates against the raw excerpt coordinate space.
func TestReviseChunkTagLikeSourceRangeValidation(t *testing.T) {
	content := "Hello </revise-text> world."
	doc, err := document.FromReader(strings.NewReader(content), "test.md", "description")
	if err != nil {
		t.Fatal(err)
	}
	captured := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = true
		respContent, _ := json.Marshal(map[string]any{
			"findings": []map[string]any{{
				"kind":          "clarification",
				"source_text":   "Hello",
				"source_range":  map[string]any{"start": 0, "end": 5},
				"principle_ids": []string{"CORE.SHORT_SENTENCE"},
				"reason":        "needs context",
				"question":      "Is this sentence clear?",
				"confidence":    0.8,
			}},
		})
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": string(respContent)}}},
		})
	}))
	defer srv.Close()

	resp, err := ReviseChunk(context.Background(), Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second}, doc, testProfile(), nil, nil, 0, len(content))
	if err != nil {
		t.Fatalf("ReviseChunk with tag-like source: %v", err)
	}
	if !captured {
		t.Fatal("expected a fake server request")
	}
	if len(resp.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(resp.Revisions))
	}
	if resp.Revisions[0].Range.StartByte != 0 || resp.Revisions[0].Range.EndByte != 5 {
		t.Fatalf("range should map to [0,5) in source bytes, got [%d,%d)",
			resp.Revisions[0].Range.StartByte, resp.Revisions[0].Range.EndByte)
	}
}

func TestReviseRepairsWrongRangeFromUniqueSourceText(t *testing.T) {
	srv := newFakeReviseServer(false, "wrong-range")
	defer srv.Close()
	response, err := Revise(context.Background(), Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second}, testDoc(), testProfile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 1 {
		t.Fatalf("revisions = %#v", response.Revisions)
	}
	got := response.Revisions[0]
	if got.SourceText != "term" || got.Range.StartByte != 11 || got.Range.EndByte != 15 {
		t.Fatalf("source evidence was not remapped: %#v", got)
	}
}

func TestReviseUsesNearestRepeatedSourceText(t *testing.T) {
	srv := newFakeReviseServer(false, "wrong-range")
	defer srv.Close()
	doc, err := document.FromReader(strings.NewReader("term term"), "test.md", "description")
	if err != nil {
		t.Fatal(err)
	}
	response, err := Revise(context.Background(), Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second}, doc, testProfile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 1 || response.Revisions[0].Range.StartByte != 0 || response.Revisions[0].Range.EndByte != 4 {
		t.Fatalf("nearest repeated evidence was not selected: %#v", response.Revisions)
	}
}

func TestNearestSourceOccurrenceRejectsDistanceTie(t *testing.T) {
	if start, ok := nearestSourceOccurrence("term__term", "term", 3); ok || start != 0 {
		t.Fatalf("expected tied occurrences to be rejected, got start=%d ok=%t", start, ok)
	}
}

func TestReviseAcceptsSingleJSONCodeFence(t *testing.T) {
	srv := newFakeReviseServer(false, "fenced")
	defer srv.Close()
	response, err := Revise(context.Background(), Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second}, testDoc(), testProfile(), nil, nil)
	if err != nil || len(response.Revisions) != 1 {
		t.Fatalf("expected fenced JSON response to pass, got %#v, err=%v", response, err)
	}
}

func TestUnwrapJSONFenceRejectsSurroundingProse(t *testing.T) {
	raw := []byte("Here is the result:\n```json\n{\"findings\":[]}\n```")
	if got := unwrapJSONFence(raw); string(got) != string(raw) {
		t.Fatalf("surrounding prose must not be unwrapped: %q", got)
	}
}

func TestReviseReportsModelErrorObject(t *testing.T) {
	srv := newFakeReviseServer(false, "model-error")
	defer srv.Close()
	_, err := Revise(context.Background(), Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second}, testDoc(), testProfile(), nil, nil)
	if !errors.Is(err, ErrInvalidModelResponse) || !strings.Contains(err.Error(), "model reported error: generation failed") || strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected model error: %v", err)
	}
}

func TestReviseWithKey(t *testing.T) {
	srv := newFakeReviseServer(true, "ok")
	defer srv.Close()

	os.Setenv("REVISE_TEST_KEY", "secret")
	defer os.Unsetenv("REVISE_TEST_KEY")

	content := "deprecated term appears here."
	doc, _ := document.FromReader(strings.NewReader(content), "test.md", "description")
	res := &profile.Resolution{
		ID: "PROFILE_ID", Version: "1",
		Rules: &profile.RulesConfig{
			Rules: []profile.Rule{
				{ID: "CORE.TERM_DISCOURAGED", Version: 1, Enabled: true},
			},
		},
	}

	cfg := Config{BaseURL: srv.URL, Model: "gpt", APIKeyEnv: "REVISE_TEST_KEY", Timeout: time.Second}
	resp, err := Revise(context.Background(), cfg, doc, res, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestReviseSendsJSONSchemaResponseFormat(t *testing.T) {
	var capturedRequest map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": `{"findings":[]}`}}},
		})
	}))
	defer srv.Close()

	doc, _ := document.FromReader(strings.NewReader("Short text."), "test.md", "description")
	res := &profile.Resolution{ID: "PROFILE_ID", Version: "1"}
	cfg := Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second, ResponseMode: "json_schema"}
	if _, err := Revise(context.Background(), cfg, doc, res, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rf, ok := capturedRequest["response_format"].(map[string]any)
	if !ok || rf["type"] != "json_schema" {
		t.Fatalf("expected response_format type json_schema, got %#v", capturedRequest["response_format"])
	}
	js, ok := rf["json_schema"].(map[string]any)
	if !ok || js["name"] != "revise_response" {
		t.Fatalf("expected json_schema name revise_response, got %#v", rf["json_schema"])
	}
	schema, _ := json.Marshal(js["schema"])
	schemaStr := string(schema)
	for _, id := range guidance.PrincipleIDs() {
		if !strings.Contains(schemaStr, id) {
			t.Fatalf("schema missing principle ID %s: %s", id, schemaStr)
		}
	}
	if strings.Contains(schemaStr, "{{PRINCIPLE_IDS}}") {
		t.Fatal("schema contains unreplaced placeholder")
	}
}

func TestValidateReviseResponseQuestionsField(t *testing.T) {
	raw := `{"questions":[{"kind":"clarification","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"unclear referent","replacement":null,"question":"What does this refer to?","confidence":0.8}]}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("expected questions field to be accepted, got: %v", err)
	}
	if len(resp.Revisions) != 1 {
		t.Fatalf("expected 1 revision from questions field, got %d", len(resp.Revisions))
	}
	if resp.Revisions[0].Kind != "clarification" {
		t.Fatalf("expected clarification, got %q", resp.Revisions[0].Kind)
	}
}

func TestValidateReviseResponseFindingsPreferredOverQuestions(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"too long","replacement":"hi","confidence":0.9}],"questions":[{"kind":"clarification","source_text":"world","source_range":{"start":0,"end":5},"principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"unclear","replacement":null,"question":"What?","confidence":0.7}]}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("expected valid response, got: %v", err)
	}
	if len(resp.Revisions) != 1 {
		t.Fatalf("expected findings to be used (not questions), got %d revisions", len(resp.Revisions))
	}
	if resp.Revisions[0].Kind != "rewrite" {
		t.Fatalf("expected rewrite from findings, got %q", resp.Revisions[0].Kind)
	}
}

func TestValidateReviseResponseAcceptsExtraTopLevelFields(t *testing.T) {
	raw := `{"findings":[],"reason":"no revisions needed","model_info":"gemma4","confidence":0.9}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("expected extra top-level fields to be silently accepted, got: %v", err)
	}
	if len(resp.Revisions) != 0 {
		t.Fatalf("expected 0 revisions, got %d", len(resp.Revisions))
	}
}

func TestValidateReviseResponseAcceptsExtraFieldsWithValidFindings(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"too long","replacement":"hi","confidence":0.9}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("expected valid response with extra fields, got: %v", err)
	}
	if len(resp.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(resp.Revisions))
	}
	if resp.Revisions[0].Replacement == nil || *resp.Revisions[0].Replacement != "hi" {
		t.Fatalf("expected replacement hi, got %#v", resp.Revisions[0].Replacement)
	}
}

func TestValidateReviseResponseStillRejectsErrorsWithExtraFields(t *testing.T) {
	// Extra top-level fields must not silence existing validation.
	raw := `{"findings":[{"kind":"rewrite","source_text":"hello","source_range":{"start":0,"end":5},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"test","confidence":0.9}],"extra":"field"}`
	_, err := ValidateReviseResponse([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "requires replacement") {
		t.Fatalf("expected requires replacement error despite extra fields, got: %v", err)
	}
}

func TestFindSourceTextNormalizedTie(t *testing.T) {
	start, end, ok := findSourceTextNormalized("abc abc", "abc", 2)
	if ok {
		t.Fatalf("expected tie to be rejected, got start=%d end=%d ok=%v", start, end, ok)
	}
}

func TestFindSourceTextNormalizedWhitespace(t *testing.T) {
	// Model says "on deploy" but document has "on\ndeploy".
	// Non-whitespace chars match: o,n,d,e,p,l,o,y.
	text := "waits on\ndeploy + CloudBees"
	source := "on deploy"
	start, end, ok := findSourceTextNormalized(text, source, 6)
	if !ok {
		t.Fatalf("expected match, got start=%d end=%d ok=%v", start, end, ok)
	}
	if text[start:end] != "on\ndeploy" {
		t.Fatalf("expected match at on\\ndeploy, got %q at [%d,%d)", text[start:end], start, end)
	}
}

func TestFindSourceTextNormalizedNoMatch(t *testing.T) {
	_, _, ok := findSourceTextNormalized("some random text here", "something else", 0)
	if ok {
		t.Fatal("expected no match for non-existent source text")
	}
}

func TestFindSourceTextNormalizedUnicode(t *testing.T) {
	text := "élève une question"
	source := "élève une"
	start, end, ok := findSourceTextNormalized(text, source, 0)
	if !ok {
		t.Fatalf("expected match for unicode text, got start=%d end=%d ok=%v", start, end, ok)
	}
	if text[start:end] != "élève une" {
		t.Fatalf("expected match élève une, got %q at [%d,%d)", text[start:end], start, end)
	}
}

func TestFindSourceTextNormalizedLeadingWhitespaceInSource(t *testing.T) {
	// Model sometimes adds/removes small words. This tests that
	// leading/trailing whitespace in source is ignored.
	text := "the quick brown fox"
	source := "  quick brown  "
	start, end, ok := findSourceTextNormalized(text, source, 0)
	if !ok {
		t.Fatalf("expected match ignoring whitespace, got start=%d end=%d ok=%v", start, end, ok)
	}
	if text[start:end] != "quick brown" {
		t.Fatalf("expected quick brown, got %q at [%d,%d)", text[start:end], start, end)
	}
}
