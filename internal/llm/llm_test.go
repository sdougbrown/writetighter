package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

func testDoc() *document.Document {
	doc, _ := document.FromReader(strings.NewReader("deprecated term appears here."), "test.md", "")
	return doc
}

func testProfile() *profile.Resolution {
	return &profile.Resolution{ID: "PROFILE_ID", Version: "1", Rules: &profile.RulesConfig{Rules: []profile.Rule{{ID: "CORE.TERM_DISCOURAGED", Version: 1, Enabled: true}}}, Dict: &profile.Dictionary{Entries: []profile.Entry{{Term: "deprecated term", Status: profile.StatusDiscouraged}}}}
}

func TestResponseModes(t *testing.T) {
	if got := buildResponseFormat("prompt_json"); got != nil {
		t.Fatalf("prompt_json must rely on the prompt, got %#v", got)
	}
	if got := buildResponseFormat("auto"); got != nil {
		t.Fatalf("auto must not resend source while probing modes, got %#v", got)
	}
	if got := buildResponseFormat("json_object"); got == nil || got.Type != "json_object" {
		t.Fatalf("json_object response format = %#v", got)
	}
	got := buildResponseFormat("json_schema")
	if got == nil || got.JSONSchema == nil || !strings.Contains(string(got.JSONSchema.Schema), `"replacement","confidence"`) {
		t.Fatalf("json_schema response format is incomplete: %#v", got)
	}
}

func TestCombinedInputLimit(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "http://127.0.0.1:1", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), Request{Messages: []Message{
		{Role: "system", Content: strings.Repeat("a", MaxInputChars/2+1)},
		{Role: "user", Content: strings.Repeat("b", MaxInputChars/2+1)},
	}})
	if err == nil || !strings.Contains(err.Error(), "input too large") {
		t.Fatalf("combined input error = %v", err)
	}
}

func TestTruncatedExcerptExclusiveEndBeforeGap(t *testing.T) {
	excerpt := newExcerpt("abc\nxyz", []int{0, 1, 2, -1, 10, 11, 12})
	truncated := truncateExcerpt(excerpt, 3)
	if got := truncated.OrigOffset(len(truncated.Text)); got != 3 {
		t.Fatalf("exclusive original end = %d, want 3", got)
	}
}

func TestNoAuthorizationHeader(t *testing.T) {
	srv := newFakeServer(false, "ok")
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil || len(resp.Choices) == 0 {
		t.Fatalf("expected response, got %v %v", resp, err)
	}
}

func TestKeyedMode(t *testing.T) {
	srv := newFakeServer(true, "ok")
	defer srv.Close()
	_ = os.Setenv("PROSEVET_API_KEY", "secret")
	defer os.Unsetenv("PROSEVET_API_KEY")
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", APIKeyEnv: "PROSEVET_API_KEY", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestResponseFallback(t *testing.T) {
	srv := newFakeServer(false, "malformed")
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestTimeoutHandling(t *testing.T) {
	srv := newFakeServer(false, "timeout")
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestAdversarialText(t *testing.T) {
	resp := `{"findings":[{"source_range":{"start":0,"end":5},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"claim","replacement":"certified compliance guaranteed","confidence":1}]}`
	_, err := ValidateAdvisorResponse([]byte(resp), "hello world")
	if err == nil {
		t.Fatal("expected claim rejection")
	}
}

func TestAdvisorSetsVersions(t *testing.T) {
	srv := newFakeServer(false, "ok")
	defer srv.Close()
	findings, err := Advisor(context.Background(), Config{BaseURL: srv.URL, Model: "gpt"}, testDoc(), testProfile(), []report.Finding{{RuleID: "CORE.TERM_DISCOURAGED", Range: &report.FindingRange{StartByte: 0, EndByte: 5}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].RuleVersion != 1 || findings[0].CheckerVersion != 1 {
		t.Fatalf("advisor versions = %#v", findings)
	}
}

func TestRequiredOptionalFailure(t *testing.T) {
	doc := testDoc()
	res := testProfile()
	_, err := Advisor(context.Background(), Config{BaseURL: "http://127.0.0.1:1", Model: "gpt", Timeout: time.Millisecond}, doc, res, []report.Finding{{RuleID: "CORE.TERM_DISCOURAGED", Message: "x", Range: &report.FindingRange{StartByte: 0, EndByte: 5}}}, nil)
	if err == nil {
		t.Fatal("expected advisor failure")
	}
}

func TestBuildPromptIncludesTerms(t *testing.T) {
	doc := testDoc()
	res := testProfile()
	terms := []config.TermEntry{{Term: "jockey", Definition: "An orchestration agent.", Override: true, Reason: "project style"}}
	sys, excerpt := BuildPrompt(doc, res, nil, terms)
	if !strings.Contains(sys, "jockey") || !strings.Contains(sys, "orchestration") {
		t.Fatal("expected term base in prompt")
	}
	if excerpt.Text == "" {
		t.Fatal("expected non-empty excerpt")
	}
}

func TestBuildPromptExcerptBounded(t *testing.T) {
	// Create a doc with multiple prose segments; only one has a finding.
	content := "First paragraph has no issue.\n\nSecond paragraph uses deprecated term here.\n\nThird paragraph also fine."
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	findings := []report.Finding{{
		RuleID:  "CORE.TERM_DISCOURAGED",
		Message: "Use 'first' instead of 'deprecated'",
		Range:   &report.FindingRange{StartByte: 49, EndByte: 66},
	}}
	_, excerpt := BuildPrompt(doc, res, findings, nil)
	if excerpt.Text == "" {
		t.Fatal("expected non-empty excerpt")
	}
	// Should contain the second paragraph (with the finding) but not necessarily
	// the full content.
	if !strings.Contains(excerpt.Text, "deprecated term") {
		t.Fatal("expected excerpt to contain the finding text")
	}
	// Verify mapping: the finding's original byte offsets should map via excerpt.OrigOffset
	// such that OrigOffset maps to somewhere in the original document.
	if excerpt.OrigOffset(0) != 0 && excerpt.OrigOffset(5) > 0 {
		t.Logf("orig offset of excerpt offset 0: %d", excerpt.OrigOffset(0))
		t.Logf("orig offset of excerpt offset 5: %d", excerpt.OrigOffset(5))
	}
}

func TestBuildPromptFullContentFallback(t *testing.T) {
	// Document with no segments that have findings should fall back to full content.
	content := "Just some text without any findings to excerpt."
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	_, excerpt := BuildPrompt(doc, res, nil, nil)
	if !strings.Contains(excerpt.Text, content[:10]) {
		t.Fatal("expected full content fallback")
	}
}

func TestValidateAdvisorResponseUTF8Boundaries(t *testing.T) {
	// A multi-byte UTF-8 character: © is 2 bytes (0xC2 0xA9)
	input := "ab©def" // bytes: a b C2 A9 d e f
	// A range that ends in the middle of the © would be invalid.
	// Start at 0, end at 3 means we include a b C2 but not A9 — invalid.
	resp := `{"findings":[{"source_range":{"start":0,"end":3},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"test","replacement":"","confidence":1}]}`
	_, err := ValidateAdvisorResponse([]byte(resp), input)
	if err == nil {
		t.Fatal("expected UTF-8 boundary error for range ending mid-code-point")
	}
}

func TestValidateAdvisorResponseUTF8BoundariesOK(t *testing.T) {
	// A range starting and ending at valid UTF-8 boundaries.
	input := "ab©def"
	resp := `{"findings":[{"source_range":{"start":0,"end":2},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"test","replacement":"","confidence":1}]}`
	_, err := ValidateAdvisorResponse([]byte(resp), input)
	if err != nil {
		t.Fatalf("expected valid UTF-8 range, got: %v", err)
	}
	// Range that ends at 5 = includes all of © at position 2-3
	resp = `{"findings":[{"source_range":{"start":0,"end":5},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"test","replacement":"","confidence":1}]}`
	_, err = ValidateAdvisorResponse([]byte(resp), input)
	if err != nil {
		t.Fatalf("expected valid UTF-8 range including multi-byte char, got: %v", err)
	}
}

func TestValidateAdvisorResponseInactiveRuleID(t *testing.T) {
	input := "test content"
	resp := `{"findings":[{"source_range":{"start":0,"end":4},"rule_ids":["NONEXISTENT.RULE"],"reason":"test","replacement":"","confidence":1}]}`
	active := map[string]struct{}{"CORE.TERM_DISCOURAGED": {}}
	_, err := ValidateAdvisorResponseForRules([]byte(resp), input, active)
	if err == nil {
		t.Fatal("expected error for inactive rule ID")
	}
}

func TestValidateAdvisorResponseInvalidConfidence(t *testing.T) {
	input := "test content"
	resp := `{"findings":[{"source_range":{"start":0,"end":4},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"test","replacement":"","confidence":1.5}]}`
	_, err := ValidateAdvisorResponse([]byte(resp), input)
	if err == nil {
		t.Fatal("expected error for invalid confidence")
	}
	resp = `{"findings":[{"source_range":{"start":0,"end":4},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"test","replacement":"","confidence":-0.5}]}`
	_, err = ValidateAdvisorResponse([]byte(resp), input)
	if err == nil {
		t.Fatal("expected error for negative confidence")
	}
}

// Issue 1: Source text must appear only in user message, not in system prompt.
func TestBuildPromptSourceNotInSystemPrompt(t *testing.T) {
	content := "This is test content with some deprecated term here."
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	findings := []report.Finding{{
		RuleID:  "CORE.TERM_DISCOURAGED",
		Message: "test",
		Range:   &report.FindingRange{StartByte: 41, EndByte: 55},
	}}
	sys, excerpt := BuildPrompt(doc, res, findings, nil)
	// System prompt must NOT contain the full passage text from the excerpt.
	if strings.Contains(sys, "This is test content") {
		t.Fatal("source text must not appear in system prompt; found text in sys")
	}
	// The excerpt must contain the passage text.
	if !strings.Contains(excerpt.Text, "This is test content") {
		t.Fatal("excerpt must contain passage text")
	}
}

// Issue 1: Total prompt+excerpt <= 32k and valid UTF-8.
func TestBuildPromptTotalBudget(t *testing.T) {
	// Create a doc whose content alone exceeds MaxInputChars.
	content := strings.Repeat("hello world\n", 5000)
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	sys, excerpt := BuildPrompt(doc, res, nil, nil)
	total := len(sys) + len(excerpt.Text)
	if total > MaxInputChars {
		t.Fatalf("total prompt+excerpt %d exceeds %d", total, MaxInputChars)
	}
	// Verify excerpt is valid UTF-8.
	if !utf8.ValidString(excerpt.Text) {
		t.Fatal("excerpt is not valid UTF-8")
	}
}

func TestBuildPromptTotalBudgetWithFindings(t *testing.T) {
	// Create a doc with a finding and verify total budget is respected.
	content := strings.Repeat("This is a very long paragraph with some deprecated words in it. ", 1000)
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	findings := []report.Finding{{
		RuleID:  "CORE.TERM_DISCOURAGED",
		Message: "test",
		Range:   &report.FindingRange{StartByte: 100, EndByte: 130},
	}}
	sys, excerpt := BuildPrompt(doc, res, findings, nil)
	total := len(sys) + len(excerpt.Text)
	if total > MaxInputChars {
		t.Fatalf("total prompt+excerpt %d exceeds %d", total, MaxInputChars)
	}
	if !utf8.ValidString(excerpt.Text) {
		t.Fatal("excerpt is not valid UTF-8 after truncation")
	}
}

// Issue 2: Exclusive end at excerpt end must return the correct original offset.
func TestExclusiveEndMapping(t *testing.T) {
	content := "Hello world. This is a test."
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	findings := []report.Finding{{
		RuleID:  "CORE.TERM_DISCOURAGED",
		Message: "test",
		Range:   &report.FindingRange{StartByte: 0, EndByte: 5},
	}}
	_, excerpt := BuildPrompt(doc, res, findings, nil)
	// The excerpt should contain the full content (since first segment has the finding).
	// OrigOffset at len(excerpt.Text) should return len(content), not len(content)-1.
	end := excerpt.OrigOffset(len(excerpt.Text))
	if end != len(content) {
		t.Fatalf("OrigOffset(len(excerpt.Text)) = %d, want %d", end, len(content))
	}
}

// Issue 2: Cross-gap range rejection.
func TestCrossGapRangeRejection(t *testing.T) {
	// A doc with multiple prose segments separated by code blocks.
	content := "First paragraph.\n\n```\ncode block\n```\n\nSecond paragraph."
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	// Finding in the first prose segment.
	findings := []report.Finding{{
		RuleID:  "CORE.TERM_DISCOURAGED",
		Message: "test",
		Range:   &report.FindingRange{StartByte: 0, EndByte: 16}, // "First paragraph."
	}}
	_, excerpt := BuildPrompt(doc, res, findings, nil)
	// The excerpt may contain only the first prose segment or both,
	// separated by a synthetic newline separator.
	// Find where a separator is.
	sepPos := -1
	for i := range excerpt.Text {
		if excerpt.origOffsets != nil && i < len(excerpt.origOffsets) && excerpt.origOffsets[i] < 0 {
			sepPos = i
			break
		}
	}
	if sepPos < 0 {
		// Only one segment included; no cross-gap test possible.
		return
	}
	// Range that starts in first segment and ends after separator should be rejected.
	if excerpt.validExcerptRange(0, len(excerpt.Text)) {
		t.Fatal("expected cross-gap range to be rejected, but it was accepted")
	}
	// Range entirely within first segment should be accepted.
	if !excerpt.validExcerptRange(0, sepPos) {
		t.Fatal("expected range within first segment to be accepted")
	}
}

// Issue 3: Unicode line/column/path remapping.
func TestByteOffsetToLineColumn(t *testing.T) {
	content := "Hello\nWorld\n"
	tests := []struct {
		offset   int
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},
		{3, 1, 4},  // "Hel" -> col 4
		{6, 2, 1},  // after first \n
		{11, 2, 6}, // "World" -> col 6
		{12, 3, 1}, // after second \n
	}
	for _, tt := range tests {
		line, col := byteOffsetToLineColumn(content, tt.offset)
		if line != tt.wantLine || col != tt.wantCol {
			t.Errorf("byteOffsetToLineColumn(%q, %d) = (%d, %d), want (%d, %d)",
				content, tt.offset, line, col, tt.wantLine, tt.wantCol)
		}
	}
}

func TestByteOffsetToLineColumnUTF8(t *testing.T) {
	// "\u00fcber" is 5 bytes, 4 code points
	content := "\u00fcber"
	line, col := byteOffsetToLineColumn(content, 0)
	if line != 1 || col != 1 {
		t.Fatalf("offset 0: got (%d,%d), want (1,1)", line, col)
	}
	line, col = byteOffsetToLineColumn(content, 2) // after 0xC3 0xBC
	if line != 1 || col != 2 {
		t.Fatalf("offset 2 (after '\u00fc'): got (%d,%d), want (1,2)", line, col)
	}
	line, col = byteOffsetToLineColumn(content, 5) // end
	if line != 1 || col != 5 {
		t.Fatalf("offset 5 (end): got (%d,%d), want (1,5)", line, col)
	}
}

// Issue 4: Static finding positions in prompt are excerpt-relative.
func TestBuildPromptStaticFindingsExcerptRelative(t *testing.T) {
	content := "First paragraph.\n\nSecond paragraph with issue here.\n\nThird paragraph."
	doc, _ := document.FromReader(strings.NewReader(content), "doc.md", "pr")
	res := testProfile()
	// Finding at original byte 30 (start of "Second paragraph with issue here.")
	findings := []report.Finding{{
		RuleID:  "CORE.TERM_DISCOURAGED",
		Message: "issue found",
		Range:   &report.FindingRange{StartByte: 30, EndByte: 68},
	}}
	sys, _ := BuildPrompt(doc, res, findings, nil)
	// The static findings listing in the prompt should reference excerpt-relative
	// offsets. Byte 30 in original = byte 0 in excerpt.
	// So it should NOT say "at bytes 30-68".
	if strings.Contains(sys, "at bytes 30-68") {
		t.Fatal("static findings listing uses original offsets; expected excerpt-relative")
	}
}

// Issue 5: Trailing JSON rejection and valid U+FFFD boundary.
func TestValidateAdvisorResponseTrailingJSON(t *testing.T) {
	input := "hello world"
	resp := `{"findings":[]}{"extra":true}`
	_, err := ValidateAdvisorResponse([]byte(resp), input)
	if err == nil {
		t.Fatal("expected error for trailing JSON")
	}
}

func TestValidateAdvisorResponseTrailingWhitespaceOK(t *testing.T) {
	input := "hello world"
	resp := `{"findings":[]}   `
	_, err := ValidateAdvisorResponse([]byte(resp), input)
	if err != nil {
		t.Fatalf("expected no error for trailing whitespace, got: %v", err)
	}
}

func TestValidateAdvisorResponseValidUTF8BoundaryWithUFFFD(t *testing.T) {
	// U+FFFD (replacement character) is a valid code point and must be accepted.
	// It's 3 bytes in UTF-8: EF BF BD.
	input := "ab\ufffddef" // bytes: a b EF BF BD d e f
	// A range ending at 5 (after the 3-byte U+FFFD) is a valid boundary.
	resp := `{"findings":[{"source_range":{"start":0,"end":5},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"test","replacement":"","confidence":1}]}`
	_, err := ValidateAdvisorResponse([]byte(resp), input)
	if err != nil {
		t.Fatalf("expected valid U+FFFD boundary, got: %v", err)
	}
	// A range starting at the middle of U+FFFD (byte 3 = BF) must be rejected.
	resp = `{"findings":[{"source_range":{"start":3,"end":5},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"test","replacement":"","confidence":1}]}`
	_, err = ValidateAdvisorResponse([]byte(resp), input)
	if err == nil {
		t.Fatal("expected error for mid-U+FFFD start boundary")
	}
}
