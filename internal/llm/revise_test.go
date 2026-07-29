package llm

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

func TestReviseResponseModes(t *testing.T) {
	if got := buildReviseResponseFormat("auto"); got != nil {
		t.Fatalf("auto must rely on prompt JSON, got %#v", got)
	}
	if got := buildReviseResponseFormat("json_object"); got == nil || got.Type != "json_object" {
		t.Fatalf("json_object revise format = %#v", got)
	}
	got := buildReviseResponseFormat("json_schema")
	if got == nil || got.JSONSchema == nil {
		t.Fatalf("missing revise JSON schema: %#v", got)
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
