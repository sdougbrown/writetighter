package document

import (
	"strings"
	"testing"
)

// testDoc creates a Document from the given text for testing.
func testDoc(text string) *Document {
	doc, _ := FromReader(strings.NewReader(text), "test.md", "description")
	return doc
}

// ---------------------------------------------------------------------------
// LexicalWordCount tests
// ---------------------------------------------------------------------------

func TestCountLexicalWordsBasic(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 1},
		{"", 0},
		{"   ", 0},
		{"hello world", 2},
		{"one two three", 3},
		{"- hello", 1},
		{"1. hello", 2},
		{"# hello", 1},
		{"> hello", 1},
		{"hello world.", 2},
		{"hello (world)", 2},
		{"hello, world!", 2},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.input)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountLexicalWordsApostrophe(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"don't", 1},
		{"can't", 1},
		{"it's", 1},
		{"we'll", 1},
		{"'tis", 1},
		{"dogs'", 1},
		{"rock 'n' roll", 3},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.input)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountLexicalWordsHyphen(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"check-in", 1},
		{"state-of-the-art", 1},
		{"well-known", 1},
		{"- prefix", 1},
		{"prefix-", 1},
		{"pre-fix", 1},
		{"v1.2.3", 1},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.input)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountLexicalWordsUnicode(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"über cool", 2},
		{"café", 1},
		{"naïve", 1},
		{"résumé", 1},
		{"über-cool", 1},
		{"München", 1},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.input)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountLexicalWordsNumbersAndDecimals(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"99", 1},
		{"3.14", 1},
		{"v1.2.3", 1},
		{"version 2.0", 2},
		{"Go 1.21", 2},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.input)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountLexicalWordsPathLike(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"/path/to/file", 3},
		{"repoCacheMu", 1},
		{"foo_bar", 2},
		{"fooBar", 1},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.input)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountLexicalWordsMarkersAreNotWords(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"- item", 1},
		{"* item", 1},
		{"1. item", 2},
		{"# heading", 1},
		{"> quote", 1},
		{"-", 0},
		{"#", 0},
		{">", 0},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.text)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestCountLexicalWordsSentenceEndPunctuation(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"Hello.", 1},
		{"Hello world.", 2},
		{"Hello world!", 2},
		{"Hello world?", 2},
		{"Hello; world.", 2},
		{"Hello: world.", 2},
	}
	for _, tc := range tests {
		got := CountLexicalWords(tc.input)
		if got != tc.want {
			t.Errorf("CountLexicalWords(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// lexicalTokens detailed tests
// ---------------------------------------------------------------------------

func TestLexicalTokens(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello", []string{"hello"}},
		{"don't", []string{"don't"}},
		{"check-in", []string{"check-in"}},
		{"repoCacheMu", []string{"repoCacheMu"}},
		{"/path/to", []string{"path", "to"}},
		{"99", []string{"99"}},
		{"-", nil},
		{"", nil},
		{"  ", nil},
		{"one two three", []string{"one", "two", "three"}},
		{"e.g.", []string{"e", "g"}},
		{"3.14", []string{"3.14"}},
		{"v1.2.3", []string{"v1.2.3"}},
		{"über-cool", []string{"über-cool"}},
		{"foo_bar", []string{"foo", "bar"}},
	}
	for _, tc := range tests {
		got := lexicalTokens(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("lexicalTokens(%q) = %v (len=%d), want %v (len=%d)",
				tc.input, got, len(got), tc.want, len(tc.want))
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("lexicalTokens(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// SentenceUnit tests (splitSentences)
// These test splitSentences directly with manually constructed blocks.
// ---------------------------------------------------------------------------

func makeBlock(content string) ProseBlock {
	return ProseBlock{
		StartByte:    0,
		EndByte:      len(content),
		AnalysisText: content,
		analysisMap:  buildIdentityMap(len(content)),
	}
}

func buildIdentityMap(n int) []int {
	m := make([]int, n+1)
	for i := 0; i <= n; i++ {
		m[i] = i
	}
	return m
}

func TestSplitSentencesSimple(t *testing.T) {
	content := "Hello world."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0].WordCount != 2 {
		t.Fatalf("expected 2 words, got %d: %q", sentences[0].WordCount, sentences[0].Text)
	}
	if sentences[0].StartByte != 0 || sentences[0].EndByte != len(content) {
		t.Fatalf("bad range: [%d,%d), want [0,%d)", sentences[0].StartByte, sentences[0].EndByte, len(content))
	}
}

func TestSplitSentencesMultiple(t *testing.T) {
	content := "Hello world. Goodbye moon."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences, got %d", len(sentences))
	}
	if sentences[0].WordCount != 2 || sentences[1].WordCount != 2 {
		t.Fatalf("expected 2 words each, got %d and %d", sentences[0].WordCount, sentences[1].WordCount)
	}
	if sentences[0].StartByte != 0 || sentences[0].EndByte != len("Hello world.") {
		t.Fatalf("first range = [%d,%d)", sentences[0].StartByte, sentences[0].EndByte)
	}
	wantSecondStart := strings.Index(content, "Goodbye")
	if sentences[1].StartByte != wantSecondStart || sentences[1].EndByte != len(content) {
		t.Fatalf("second range = [%d,%d), want [%d,%d)", sentences[1].StartByte, sentences[1].EndByte, wantSecondStart, len(content))
	}
}

func TestSplitSentencesMultiSentenceOnOneLine(t *testing.T) {
	content := "Go is fast. Rust is safe. Choose wisely."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 3 {
		t.Fatalf("expected 3 sentences, got %d", len(sentences))
	}
	expectedCounts := []int{3, 3, 2}
	for i, s := range sentences {
		if s.WordCount != expectedCounts[i] {
			t.Errorf("sentence %d: expected %d words, got %d: %q",
				i, expectedCounts[i], s.WordCount, s.Text)
		}
	}
}

func TestSplitSentencesAbbreviationProtection(t *testing.T) {
	content := "Use e.g. Docker or Podman. Pick one."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences (abbreviation should not break), got %d", len(sentences))
	}
}

func TestSplitSentencesSemicolonPreserved(t *testing.T) {
	content := "First part; second part. New sentence."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences (semicolon is not a terminator), got %d", len(sentences))
	}
	if sentences[0].WordCount != 4 {
		t.Fatalf("expected 4 words in first sentence (semicolon-preserved), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

func TestSplitSentencesExclamationAndQuestion(t *testing.T) {
	content := "Stop! Are you sure? Let's go."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 3 {
		t.Fatalf("expected 3 sentences, got %d", len(sentences))
	}
}

func TestSplitSentencesDecimalNotSplitByPeriod(t *testing.T) {
	content := "Use Go 1.21. It is fast."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences (version dot not a break), got %d", len(sentences))
	}
}

func TestSplitSentencesNoTrailingPunctuation(t *testing.T) {
	content := "Hello world"
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
}

func TestSplitSentencesEmptyBlock(t *testing.T) {
	content := ""
	block := ProseBlock{
		AnalysisText: "",
	}
	sentences := SentenceUnits(block, content)
	if len(sentences) != 0 {
		t.Fatalf("expected 0 sentences for empty block, got %d", len(sentences))
	}
}

func TestSplitSentencesOnlyWhitespace(t *testing.T) {
	content := "   \t  \n  "
	block := ProseBlock{
		AnalysisText: "   \t  \n  ",
	}
	sentences := SentenceUnits(block, content)
	if len(sentences) != 0 {
		t.Fatalf("expected 0 sentences for whitespace-only block, got %d", len(sentences))
	}
}

// ---------------------------------------------------------------------------
// ProseBlock construction tests (AnalyzeProse)
// ---------------------------------------------------------------------------

func TestAnalyzeProseSimple(t *testing.T) {
	doc := testDoc("Hello world.")
	blocks := AnalyzeProse(doc)
	if len(blocks) == 0 {
		t.Fatal("expected at least one prose block")
	}
	if blocks[0].AnalysisText != "Hello world." {
		t.Fatalf("unexpected analysis text: %q", blocks[0].AnalysisText)
	}
}

func TestAnalyzeProseMultiSentenceLine(t *testing.T) {
	text := "Go is fast. Rust is safe. Choose wisely."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 3 {
		t.Fatalf("expected 3 sentences, got %d", len(sentences))
	}
	expectedCounts := []int{3, 3, 2}
	for i, s := range sentences {
		if s.WordCount != expectedCounts[i] {
			t.Errorf("sentence %d: expected %d words, got %d: %q",
				i, expectedCounts[i], s.WordCount, s.Text)
		}
	}
}

func TestAnalyzeProseInlineCodeBridge(t *testing.T) {
	text := "The `repoCacheMu` variable is set."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// Words: "The", "variable", "is", "set" = 4 (inline code bridged not counted)
	if sentences[0].WordCount != 4 {
		t.Fatalf("expected 4 words, got %d: %q", sentences[0].WordCount, blocks[0].AnalysisText)
	}
}

// Regression: PR #39 pattern - inline code mid-sentence
func TestAnalyzeProseInlineCodePR39(t *testing.T) {
	text := "This long sentence has an inline code span `repoCacheMu` that should not break the count of words in this very long running sentence."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// 22 lexical words (inline code `repoCacheMu` bridged as space, not counted)
	if sentences[0].WordCount != 22 {
		t.Fatalf("expected 22 words (PR #39 regression, inline code excluded), got %d: %q",
			sentences[0].WordCount, blocks[0].AnalysisText)
	}
}

// Regression: PR #36 pattern - list item marker excluded
func TestAnalyzeProseListItemPR36(t *testing.T) {
	text := "- Run the test suite."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	yesContains(t, blocks[0].AnalysisText, "Run", "analysis text should contain content after marker")
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// "Run", "the", "test", "suite" = 4
	if sentences[0].WordCount != 4 {
		t.Fatalf("expected 4 words ('-' marker not counted), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

func yesContains(t *testing.T, s, substr string, msg string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Fatalf("%s: %q does not contain %q", msg, s, substr)
	}
}

// Heading test
func TestAnalyzeProseHeading(t *testing.T) {
	text := "# Title\n\nSome content."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) < 1 {
		t.Fatalf("expected >= 1 prose block, got %d", len(blocks))
	}
}

// Blockquote
func TestAnalyzeProseBlockquote(t *testing.T) {
	text := "> A wise quote."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0].WordCount != 3 {
		t.Fatalf("expected 3 words (> marker not counted), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

// Code blocks break prose blocks
func TestAnalyzeProseCodeBlockBreaks(t *testing.T) {
	text := "Before.\n```\ncode\n```\nAfter."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) < 2 {
		t.Fatalf("expected at least 2 prose blocks (code block breaks), got %d", len(blocks))
	}
}

// Wrapped lines join with space
func TestAnalyzeProseWrappedLines(t *testing.T) {
	text := "This is a long\nwrapped line."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block for wrapped lines, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0].WordCount != 6 {
		t.Fatalf("expected 6 words (wrapped line joined), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

func TestAnalyzeProseBlankLineBreak(t *testing.T) {
	text := "First paragraph.\n\nSecond paragraph."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 prose blocks (blank line break), got %d", len(blocks))
	}
}

// Paragraph with inline code and link destinations
func TestAnalyzeProseLinkDestBridge(t *testing.T) {
	text := "Visit [example](https://example.com) for more."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) < 1 {
		t.Fatalf("expected at least 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) < 1 {
		t.Fatal("expected at least 1 sentence")
	}
	if sentences[0].WordCount < 4 {
		t.Fatalf("expected at least 4 words, got %d: %q", sentences[0].WordCount, sentences[0].Text)
	}
}

// ---------------------------------------------------------------------------
// Byte range tests
// ---------------------------------------------------------------------------

func TestSentenceByteRanges(t *testing.T) {
	content := "First sentence. Second sentence."
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences, got %d", len(sentences))
	}
	expectedFirst := "First sentence."
	gotFirst := content[sentences[0].StartByte:sentences[0].EndByte]
	if gotFirst != expectedFirst {
		t.Fatalf("first sentence range: got %q [%d,%d), want %q",
			gotFirst, sentences[0].StartByte, sentences[0].EndByte, expectedFirst)
	}
	expectedSecond := "Second sentence."
	gotSecond := content[sentences[1].StartByte:sentences[1].EndByte]
	if gotSecond != expectedSecond {
		t.Fatalf("second sentence range: got %q [%d,%d), want %q",
			gotSecond, sentences[1].StartByte, sentences[1].EndByte, expectedSecond)
	}
}

func TestSentenceByteRangeInlineCode(t *testing.T) {
	content := "Run `make test` to verify."
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0].StartByte != 0 || sentences[0].EndByte != len(content) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)",
			len(content), sentences[0].StartByte, sentences[0].EndByte)
	}
}

// ---------------------------------------------------------------------------
// Unicode position tests
// ---------------------------------------------------------------------------

func TestUnicodePositions(t *testing.T) {
	content := "über cool."
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0].StartLine != 1 || sentences[0].StartColumn != 1 {
		t.Fatalf("expected Line=1 Col=1, got Line=%d Col=%d",
			sentences[0].StartLine, sentences[0].StartColumn)
	}
	if sentences[0].EndByte != len(content) {
		t.Fatalf("expected end byte %d, got %d", len(content), sentences[0].EndByte)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestAnalyzeProseEmptyDocument(t *testing.T) {
	doc := testDoc("")
	blocks := AnalyzeProse(doc)
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty document, got %d", len(blocks))
	}
}

func TestAnalyzeProseNilDocument(t *testing.T) {
	blocks := AnalyzeProse(nil)
	if blocks != nil {
		t.Fatalf("expected nil for nil document, got %v", blocks)
	}
}

// Test that we handle the exact PR #36 text pattern
func TestPR36RegressionExact(t *testing.T) {
	text := "- Run the test suite and verify the output passes all checks."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// 11 words (no dash counted)
	if sentences[0].WordCount != 11 {
		t.Fatalf("expected 11 words (PR #36 regression, dash not counted), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

// Test abbreviations in sentence splitting
func TestAbbreviationNotBreakingSentence(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"Use e.g. Docker. Done.", 2},
		{"Do i.e. fix it. OK.", 2},
		{"Use etc. carefully. Done.", 2},
		{"A vs. B comparison. Done.", 2},
	}
	for _, tc := range tests {
		doc := testDoc(tc.input)
		blocks := AnalyzeProse(doc)
		if len(blocks) == 0 {
			t.Fatalf("no blocks for %q", tc.input)
		}
		sentences := SentenceUnits(blocks[0], doc.Content)
		if len(sentences) != tc.want {
			t.Errorf("abbreviation test %q: expected %d sentences, got %d",
				tc.input, tc.want, len(sentences))
		}
	}
}

// ---------------------------------------------------------------------------
// New regression tests for defect fixes
// ---------------------------------------------------------------------------

// Adjacent bullet items without blank lines yield separate blocks.
func TestAdjacentBulletsSeparateBlocks(t *testing.T) {
	text := "- First item\n- Second item\n"
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks for adjacent bullets, got %d", len(blocks))
	}
	if blocks[0].AnalysisText != "First item" {
		t.Fatalf("block 0 analysis: got %q, want %q", blocks[0].AnalysisText, "First item")
	}
	if blocks[1].AnalysisText != "Second item" {
		t.Fatalf("block 1 analysis: got %q, want %q", blocks[1].AnalysisText, "Second item")
	}
}

// Adjacent ordered list items yield separate blocks.
func TestMarkdownTableRowsStaySeparate(t *testing.T) {
	doc := testDoc("| Name | Description |\n| --- | --- |\n| alpha | First item |\n| beta | Second item |")
	blocks := AnalyzeProse(doc)
	if len(blocks) != 4 {
		t.Fatalf("table blocks = %d, want 4", len(blocks))
	}
	for i, block := range blocks {
		if block.StartLine != i+1 || block.EndLine != i+1 {
			t.Fatalf("block %d lines = %d-%d", i, block.StartLine, block.EndLine)
		}
	}
}

func TestThematicBreakSplitsParagraphs(t *testing.T) {
	doc := testDoc("First paragraph\n---\nSecond paragraph")
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
}

func TestAdjacentOrderedItemsSeparateBlocks(t *testing.T) {
	text := "1. Do this.\n2. Do that.\n"
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks for adjacent ordered items, got %d", len(blocks))
	}
	if blocks[0].AnalysisText != "Do this." {
		t.Fatalf("block 0 analysis: got %q, want %q", blocks[0].AnalysisText, "Do this.")
	}
	if blocks[1].AnalysisText != "Do that." {
		t.Fatalf("block 1 analysis: got %q, want %q", blocks[1].AnalysisText, "Do that.")
	}
}

// At least three consecutive paragraphs all split.
func TestThreeParagraphsAllSplit(t *testing.T) {
	text := "First para.\n\nSecond para.\n\nThird para.\n"
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks for three paragraphs, got %d", len(blocks))
	}
	if blocks[0].AnalysisText != "First para." {
		t.Fatalf("block 0: got %q", blocks[0].AnalysisText)
	}
	if blocks[1].AnalysisText != "Second para." {
		t.Fatalf("block 1: got %q", blocks[1].AnalysisText)
	}
	if blocks[2].AnalysisText != "Third para." {
		t.Fatalf("block 2: got %q", blocks[2].AnalysisText)
	}
}

// Heading immediately followed by paragraph splits.
func TestHeadingThenParagraphSplit(t *testing.T) {
	text := "# Title\n\nContent here.\n"
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (heading + para), got %d", len(blocks))
	}
	if blocks[0].AnalysisText != "Title" {
		t.Fatalf("block 0 analysis: got %q, want %q", blocks[0].AnalysisText, "Title")
	}
	if blocks[1].AnalysisText != "Content here." {
		t.Fatalf("block 1 analysis: got %q, want %q", blocks[1].AnalysisText, "Content here.")
	}
}

// Exact ranges/columns after marker, inline code, wrapped newlines, Unicode.
func TestExactSourceMappingAfterMarker(t *testing.T) {
	// Bullet item: "- Run the test suite." → marker "- " stripped,
	// sentence should map to original bytes [2,20) (after "- ")
	text := "- Run the test suite."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	if s.StartByte != 2 {
		t.Fatalf("expected StartByte=2 (after '- '), got %d", s.StartByte)
	}
	if s.EndByte != len(text) {
		t.Fatalf("expected EndByte=%d, got %d", len(text), s.EndByte)
	}
	if s.StartColumn != 3 {
		t.Fatalf("expected StartColumn=3 (col 3 = 'R'), got %d", s.StartColumn)
	}
}

func TestExactSourceMappingInlineCode(t *testing.T) {
	// Sentence spanning inline code must report complete original range,
	// including the excluded code region.
	text := "Use `code` here."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	if s.StartByte != 0 {
		t.Fatalf("expected StartByte=0, got %d", s.StartByte)
	}
	if s.EndByte != len(text) {
		t.Fatalf("expected EndByte=%d (full range including inline code), got %d", len(text), s.EndByte)
	}
	if s.StartLine != 1 || s.StartColumn != 1 {
		t.Fatalf("expected Line=1 Col=1, got Line=%d Col=%d", s.StartLine, s.StartColumn)
	}
	// Verify the original range slices to the complete text.
	if doc.Content[s.StartByte:s.EndByte] != text {
		t.Fatalf("original slice mismatch: %q", doc.Content[s.StartByte:s.EndByte])
	}
}

func TestExactSourceMappingWrappedNewlines(t *testing.T) {
	text := "This is a line\nthat continues."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	// Full original range
	if s.StartByte != 0 || s.EndByte != len(text) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(text), s.StartByte, s.EndByte)
	}
	if s.StartLine != 1 || s.StartColumn != 1 {
		t.Fatalf("expected Line=1 Col=1, got Line=%d Col=%d", s.StartLine, s.StartColumn)
	}
}

func TestWrappedLineSentenceBoundaryMapping(t *testing.T) {
	text := "First sentence.\nSecond sentence."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 2 {
		t.Fatalf("sentences = %d, want 2", len(sentences))
	}
	firstEnd := strings.IndexByte(text, '\n')
	secondStart := firstEnd + 1
	if sentences[0].EndByte != firstEnd {
		t.Fatalf("first end = %d, want %d", sentences[0].EndByte, firstEnd)
	}
	if sentences[1].StartByte != secondStart || sentences[1].StartLine != 2 || sentences[1].StartColumn != 1 {
		t.Fatalf("second start = byte %d line %d col %d", sentences[1].StartByte, sentences[1].StartLine, sentences[1].StartColumn)
	}
}

func TestExactSourceMappingUnicode(t *testing.T) {
	text := "über cool."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	if s.StartByte != 0 || s.EndByte != len(text) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(text), s.StartByte, s.EndByte)
	}
	if s.StartLine != 1 || s.StartColumn != 1 {
		t.Fatalf("expected Line=1 Col=1, got Line=%d Col=%d", s.StartLine, s.StartColumn)
	}
}

// PR #36 marker excluded test variant
func TestPR36MarkerExcludedFromRange(t *testing.T) {
	text := "- Run checks."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	if s.StartByte != 2 {
		t.Fatalf("expected StartByte=2 (after '- '), got %d", s.StartByte)
	}
	got := doc.Content[s.StartByte:s.EndByte]
	if got != "Run checks." {
		t.Fatalf("sentence content mismatch: got %q, want %q", got, "Run checks.")
	}
	if s.WordCount != 2 {
		t.Fatalf("expected 2 words, got %d", s.WordCount)
	}
}

// PR #39 full sentence remains one sentence while inline code excluded.
func TestPR39FullSentenceRange(t *testing.T) {
	text := "This long sentence has an inline code span `repoCacheMu` that should not break the count of words in this very long running sentence."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	// Full original range, including the inline code
	if s.StartByte != 0 || s.EndByte != len(text) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(text), s.StartByte, s.EndByte)
	}
	// Inline code excluded from word count
	if s.WordCount != 22 {
		t.Fatalf("expected 22 words (inline code excluded), got %d", s.WordCount)
	}
}

// --------------------------------------------------------------------------
// Requirement 2: Dotted decimals/versions as single lexical tokens
// --------------------------------------------------------------------------

func TestLexicalTokensDecimalVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"3.14", []string{"3.14"}},
		{"v1.2.3", []string{"v1.2.3"}},
		{"Effect 4.0.0-beta.97", []string{"Effect", "4.0.0-beta.97"}},
		{"Go 1.21", []string{"Go", "1.21"}},
		{"version 2.0", []string{"version", "2.0"}},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := lexicalTokens(tc.input)
			if len(got) != len(tc.expected) {
				t.Errorf("lexicalTokens(%q) = %v, want %v", tc.input, got, tc.expected)
				return
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("lexicalTokens(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.expected[i])
				}
			}
		})
	}
}

// Sentence-level boundary test for dotted version not breaking the sentence
func TestSplitSentencesDecimalVersionNotBreak(t *testing.T) {
	content := "Use Go 1.21 for this. It is fast."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences, got %d", len(sentences))
	}
	t.Logf("first sentence: %q, word count: %d", sentences[0].Text, sentences[0].WordCount)
}

// --------------------------------------------------------------------------
// Requirement 3: Multi-backtick inline code
// --------------------------------------------------------------------------

func TestAnalyzeProseMultiBacktickCode(t *testing.T) {
	// Multi-backtick inline code with `content` containing single backticks
	text := "Use `` `cmd` `` for running commands."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// Words: Use, for, running, commands = 4 (inline code excluded)
	if sentences[0].WordCount != 4 {
		t.Fatalf("expected 4 words (multi-backtick inline code excluded), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

func TestAnalyzeProseMultiBacktickTriple(t *testing.T) {
	// Triple backtick inline code with content containing shorter backtick runs
	text := "Use ``` ``inner`` ``` for nesting."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// Words: Use, for, nesting = 3
	if sentences[0].WordCount != 3 {
		t.Fatalf("expected 3 words (triple-backtick code excluded), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
	// Source range must include entire span
	s := sentences[0]
	if s.StartByte != 0 || s.EndByte != len(text) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(text), s.StartByte, s.EndByte)
	}
}

func TestSplitSentencesMultiBacktickCodePreserveRange(t *testing.T) {
	content := "The `` `code` `` span ends correctly."
	block := makeBlock(content)
	block.analysisMap = buildIdentityMap(len(content))
	sentences := SentenceUnits(block, content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0].StartByte != 0 || sentences[0].EndByte != len(content) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(content), sentences[0].StartByte, sentences[0].EndByte)
	}
}

// --------------------------------------------------------------------------
// Requirement 6: Exclude Markdown autolinks from lexical counts
// --------------------------------------------------------------------------

func TestAnalyzeProseAutolinkExcluded(t *testing.T) {
	text := "Visit <https://example.com/a-b> for details."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// Words: Visit, for, details = 3 (autolink excluded)
	if sentences[0].WordCount != 3 {
		t.Fatalf("expected 3 words (autolink excluded), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
	s := sentences[0]
	// Full original range preserved
	if s.StartByte != 0 || s.EndByte != len(text) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(text), s.StartByte, s.EndByte)
	}
}

func TestAnalyzeProseMailtoAutolink(t *testing.T) {
	text := "Contact <mailto:x@example.com> now."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// Words: Contact, now = 2 (mailto link excluded)
	if sentences[0].WordCount != 2 {
		t.Fatalf("expected 2 words (mailto autolink excluded), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

// --------------------------------------------------------------------------
// Requirement 1: Trailing excluded spans preserve sentence range.
// For `Use ``cmd```, the sentence range must end after the excluded span.
// --------------------------------------------------------------------------

func TestSentenceRangeAfterTrailingCode(t *testing.T) {
	// Use AnalyzeProse so the inline code span is bridged to a space.
	content := "Use `cmd`"
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	// Range should end at the end of content (after the trailing code),
	// not trimmed before it.
	if s.EndByte != len(content) {
		t.Fatalf("expected EndByte %d (full range including trailing code), got %d", len(content), s.EndByte)
	}
	if s.WordCount != 1 {
		t.Fatalf("expected 1 word (Use), got %d: %q", s.WordCount, s.Text)
	}
}

func TestSentenceRangeAfterTrailingLink(t *testing.T) {
	// Use AnalyzeProse so link destination is bridged to a space.
	content := "Visit [docs](https://x)"
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	if s.EndByte != len(content) {
		t.Fatalf("expected EndByte %d (full range including trailing link), got %d", len(content), s.EndByte)
	}
	if s.WordCount != 2 {
		t.Fatalf("expected 2 words (Visit and docs/link-text), got %d: %q", s.WordCount, s.Text)
	}
}

// --------------------------------------------------------------------------
// Requirement 7: Contextual etc. behavior
// --------------------------------------------------------------------------

func TestAbbreviationEtcFollowedByCapitalized(t *testing.T) {
	// "etc." followed by a capitalized word — etc. is contextual.
	content := "Use tools etc. Docker is one."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	// etc. followed by "Docker" (uppercase) terminates the sentence
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences (etc. terminates before capitalized), got %d", len(sentences))
	}
}

func TestAbbreviationEtcFollowedByNewSentence(t *testing.T) {
	// "etc." followed by a capitalized word: etc. terminates.
	content := "Bring tools etc. Next, continue."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences (etc. terminates), got %d", len(sentences))
	}
	if sentences[0].WordCount != 3 {
		t.Fatalf("expected 3 words in first sentence (Bring, tools, etc.), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

func TestAbbreviationEtcAtEndOfSentence(t *testing.T) {
	content := "Bring the tools, etc. Next we continue."
	block := makeBlock(content)
	sentences := SentenceUnits(block, content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences, got %d", len(sentences))
	}
	// Words: Bring, the, tools, etc = 4
	if sentences[0].WordCount != 4 {
		t.Fatalf("expected 4 words in first sentence (Bring, the, tools, etc.), got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

// --------------------------------------------------------------------------
// Requirement 8: Fenced code, front-matter, HTML exclusion tests
// --------------------------------------------------------------------------

func TestAnalyzeProseFencedCodeExactBlock(t *testing.T) {
	text := "Before.\n```\ncode block\n```\nAfter."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	// Should produce exactly 2 blocks (before and after code fence)
	if len(blocks) != 2 {
		t.Fatalf("expected exactly 2 prose blocks (fenced code excluded), got %d", len(blocks))
	}
	if !strings.Contains(blocks[0].AnalysisText, "Before.") {
		t.Errorf("block 0 should contain 'Before.', got %q", blocks[0].AnalysisText)
	}
	if !strings.Contains(blocks[1].AnalysisText, "After.") {
		t.Errorf("block 1 should contain 'After.', got %q", blocks[1].AnalysisText)
	}
}

func TestAnalyzeProseFrontMatterExcluded(t *testing.T) {
	text := "---\ntitle: Test\n---\n\nContent here."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	// Front matter should be excluded, leaving one prose block for "Content here."
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block (front matter excluded), got %d", len(blocks))
	}
	if !strings.Contains(blocks[0].AnalysisText, "Content here.") {
		t.Errorf("block should contain 'Content here.', got %q", blocks[0].AnalysisText)
	}
}

func TestAnalyzeProseInlineHTMLRemainsProse(t *testing.T) {
	// Inline HTML like <em>Important.</em> must remain as prose, not hidden
	text := "This is <em>very</em> important."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 prose block, got %d", len(blocks))
	}
	// The block should contain the full text
	if !strings.Contains(blocks[0].AnalysisText, "This") {
		t.Errorf("block should contain prose content, got %q", blocks[0].AnalysisText)
	}
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	// Inline tags are syntax, while their text remains prose.
	if sentences[0].WordCount != 4 {
		t.Fatalf("expected 4 prose words with inline tags excluded, got %d: %q",
			sentences[0].WordCount, sentences[0].Text)
	}
}

func TestAnalyzeProseBlockHTMLExcluded(t *testing.T) {
	// Block-level HTML <div> should be excluded. Use blank line for separation.
	text := "Before.\n<div>\ncontent\n</div>\n\nAfter."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 {
		t.Fatalf("expected exactly 2 prose blocks (HTML block excluded), got %d", len(blocks))
	}
	if !strings.Contains(blocks[0].AnalysisText, "Before.") {
		t.Errorf("block 0 should contain 'Before.', got %q", blocks[0].AnalysisText)
	}
	if !strings.Contains(blocks[1].AnalysisText, "After.") {
		t.Errorf("block 1 should contain 'After.', got %q", blocks[1].AnalysisText)
	}
	for _, b := range blocks {
		if strings.Contains(b.AnalysisText, "<div>") || strings.Contains(b.AnalysisText, "content") {
			t.Errorf("HTML block content leaked: %q", b.AnalysisText)
		}
	}
}

func TestAnalyzeProseHTMLCommentExcluded(t *testing.T) {
	text := "Before.\n<!-- comment -->\nAfter."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 {
		t.Fatalf("expected exactly 2 prose blocks (HTML comment excluded), got %d", len(blocks))
	}
	if !strings.Contains(blocks[0].AnalysisText, "Before.") {
		t.Errorf("block 0 should contain 'Before.', got %q", blocks[0].AnalysisText)
	}
	if !strings.Contains(blocks[1].AnalysisText, "After.") {
		t.Errorf("block 1 should contain 'After.', got %q", blocks[1].AnalysisText)
	}
}

// --------------------------------------------------------------------------
// Requirement 9: Link-destination tests with exact word counts and source ranges
// --------------------------------------------------------------------------

func TestClosedHTMLBlockDoesNotConsumeFollowingProse(t *testing.T) {
	text := "Before.\n<div>hidden</div>\nAfter."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	if len(blocks) != 2 || blocks[1].AnalysisText != "After." {
		t.Fatalf("blocks = %#v", blocks)
	}
}

func TestBareEmailAutolinkExcluded(t *testing.T) {
	content := "Email <user@example.com> now."
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 || sentences[0].WordCount != 2 {
		t.Fatalf("sentences = %#v", sentences)
	}
	if sentences[0].StartByte != 0 || sentences[0].EndByte != len(content) {
		t.Fatalf("range = [%d,%d)", sentences[0].StartByte, sentences[0].EndByte)
	}
}

func TestLinkDestinationExactWordCountAndRange(t *testing.T) {
	// Use AnalyzeProse so the link destination is bridged to a space.
	content := "Visit [example](https://example.com) now."
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	// Words: Visit, example, now = 3 (link destination excluded)
	if s.WordCount != 3 {
		t.Fatalf("expected 3 words (Visit, example, now), got %d: %q", s.WordCount, s.Text)
	}
	if s.StartByte != 0 || s.EndByte != len(content) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(content), s.StartByte, s.EndByte)
	}
	if s.StartLine != 1 || s.StartColumn != 1 {
		t.Fatalf("expected Line=1 Col=1, got Line=%d Col=%d", s.StartLine, s.StartColumn)
	}
}

func TestLinkDestinationNestedParentheses(t *testing.T) {
	// Use AnalyzeProse so the link destination is bridged to a space.
	content := "See [wiki](https://en.wikipedia.org/wiki/Go_(programming_language)) here."
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	// Words: See, wiki, here = 3 (link destination with nested parens excluded)
	if s.WordCount != 3 {
		t.Fatalf("expected 3 words (nested parens link excluded), got %d: %q", s.WordCount, s.Text)
	}
	// Full range preserved including the entire link destination with nested parens
	if s.StartByte != 0 || s.EndByte != len(content) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(content), s.StartByte, s.EndByte)
	}
}

// --------------------------------------------------------------------------
// Requirement 10: Exact exclusive end line/column assertions for markers,
// wrapped lines, links, Unicode, and excluded spans.
// --------------------------------------------------------------------------

func TestExactEndLineColumnOrderedItemMarker(t *testing.T) {
	text := "1. Do this step."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	// Marker "1. " is stripped; sentence starts at byte 3, column 4
	if s.StartByte != 3 {
		t.Fatalf("expected StartByte=3 (after '1. '), got %d", s.StartByte)
	}
	if s.StartColumn != 4 {
		t.Fatalf("expected StartColumn=4, got %d", s.StartColumn)
	}
	// offsetToPos exclusive-end: 16 runes → col=17
	if s.EndLine != 1 || s.EndColumn != 17 {
		t.Fatalf("expected EndLine=1, EndColumn=17, got Line=%d, Col=%d", s.EndLine, s.EndColumn)
	}
}

func TestExactEndLineColumnBlockquote(t *testing.T) {
	text := "> Important note."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	// Marker "> " stripped, sentence starts at byte 2, column 3
	if s.StartByte != 2 {
		t.Fatalf("expected StartByte=2, got %d", s.StartByte)
	}
	// offsetToPos exclusive-end: 17 runes → col=18
	if s.EndColumn != 18 {
		t.Fatalf("expected EndColumn=18, got %d", s.EndColumn)
	}
}

func TestExactEndLineColumnWrappedLines(t *testing.T) {
	text := "First line.\nSecond line."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences, got %d", len(sentences))
	}
	// First sentence ends at end of first line
	firstEnd := len("First line.") // 11
	if sentences[0].EndByte != firstEnd {
		t.Fatalf("expected EndByte=%d for first sentence, got %d", firstEnd, sentences[0].EndByte)
	}
	if sentences[0].EndLine != 1 {
		t.Fatalf("expected EndLine=1 for first sentence, got %d", sentences[0].EndLine)
	}
	// Second sentence starts at beginning of second line, col 1
	if sentences[1].StartByte != firstEnd+1 {
		t.Fatalf("expected StartByte=%d for second sentence, got %d", firstEnd+1, sentences[1].StartByte)
	}
	if sentences[1].StartLine != 2 || sentences[1].StartColumn != 1 {
		t.Fatalf("expected second sentence Line=2 Col=1, got Line=%d Col=%d", sentences[1].StartLine, sentences[1].StartColumn)
	}
	if sentences[1].EndLine != 2 || sentences[1].EndColumn != 13 {
		t.Fatalf("expected second sentence EndLine=2 EndColumn=13, got Line=%d Col=%d", sentences[1].EndLine, sentences[1].EndColumn)
	}
}

func TestExactEndLineColumnUnicode(t *testing.T) {
	text := "über cool. nächster satz."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 2 {
		t.Fatalf("expected 2 sentences, got %d", len(sentences))
	}
	// First sentence: "über cool." — ü is 1 column (rune) but 2 bytes
	// Run positions: ü(1)b(2)e(3)r(4) (5)c(6)o(7)o(8)l(9).(10)
	firstEnd := len("über cool.")
	if sentences[0].EndByte != firstEnd {
		t.Fatalf("expected EndByte=%d, got %d", firstEnd, sentences[0].EndByte)
	}
	// offsetToPos exclusive-end: 10 runes → col=11
	if sentences[0].EndColumn != 11 {
		t.Fatalf("expected EndColumn=11 for 'über cool.', got %d", sentences[0].EndColumn)
	}
	// Second sentence: "nächster satz." starts at firstEnd+1 for space
	secondStart := firstEnd + 1
	if sentences[1].StartByte != secondStart {
		t.Fatalf("expected StartByte=%d, got %d", secondStart, sentences[1].StartByte)
	}
	// Start of second sentence: space at byte 11 and then 'n' at byte 12 → col=12
	if sentences[1].StartColumn != 12 {
		t.Fatalf("expected StartColumn=12, got %d", sentences[1].StartColumn)
	}
}

func TestExactEndLineColumnInlineCodeExcluded(t *testing.T) {
	text := "Run `make test` now."
	doc := testDoc(text)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	if s.StartLine != 1 || s.StartColumn != 1 {
		t.Fatalf("expected StartLine=1, StartColumn=1, got Line=%d Col=%d", s.StartLine, s.StartColumn)
	}
	// End at end of line: "Run `make test` now." = 20 chars
	// But with backtick content bridged, byte range still covers full original
	if s.EndByte != len(text) {
		t.Fatalf("expected EndByte=%d, got %d", len(text), s.EndByte)
	}
}

func TestExactEndLineColumnLinkExcluded(t *testing.T) {
	content := "Visit [site](https://x.com) now."
	doc := testDoc(content)
	blocks := AnalyzeProse(doc)
	sentences := SentenceUnits(blocks[0], doc.Content)
	if len(sentences) != 1 {
		t.Fatalf("expected 1 sentence, got %d", len(sentences))
	}
	s := sentences[0]
	if s.StartByte != 0 || s.EndByte != len(content) {
		t.Fatalf("expected full range [0,%d), got [%d,%d)", len(content), s.StartByte, s.EndByte)
	}
	if s.StartLine != 1 || s.StartColumn != 1 {
		t.Fatalf("expected StartLine=1 Col=1, got Line=%d Col=%d", s.StartLine, s.StartColumn)
	}
	// Words: Visit, site, now = 3
	if s.WordCount != 3 {
		t.Fatalf("expected 3 words (link excluded), got %d: %q", s.WordCount, s.Text)
	}
}

// Benchmark
func BenchmarkProseAnalysis(b *testing.B) {
	text := `This is the first sentence. This is the second sentence with ` + "`" + `some code` + "`" + ` inside. And here is a third sentence with more words for counting purposes.
And this is a wrapped line that continues the paragraph. Yet another sentence here.

New paragraph begins here. With more content. And inline ` + "`" + `code` + "`" + ` bridges.`
	for i := 0; i < b.N; i++ {
		doc, err := FromReader(strings.NewReader(text), "bench.md", "description")
		if err != nil {
			b.Fatal(err)
		}
		blocks := AnalyzeProse(doc)
		for _, block := range blocks {
			_ = SentenceUnits(block, doc.Content)
		}
	}
}
