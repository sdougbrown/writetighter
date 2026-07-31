package document

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractHTMLVisibleTextTracksInlineAndEntitySpans(t *testing.T) {
	source := `<p>Use <em>safe</em> &amp; clear text.</p>`
	got := ExtractHTMLVisibleText(source)
	if got.Text != "Use safe & clear text.\n" {
		t.Fatalf("text = %q", got.Text)
	}

	start := strings.Index(got.Text, "safe & clear")
	spans := (&Document{Format: FormatHTML, Content: source, Analysis: got.Text, Projection: got.Segments}).SourceSpansForAnalysisRange(start, start+len("safe & clear"))
	if len(spans) < 3 {
		t.Fatalf("spans = %#v, want text/entity/text provenance", spans)
	}
	clearStart := strings.Index(got.Text, "clear")
	clearSpan := (&Document{Format: FormatHTML, Content: source, Analysis: got.Text, Projection: got.Segments}).SourceSpansForAnalysisRange(clearStart, clearStart+len("clear"))
	if len(clearSpan) != 1 || source[clearSpan[0].StartByte:clearSpan[0].EndByte] != "clear" {
		t.Fatalf("literal subspan = %#v", clearSpan)
	}
	for _, want := range []string{"safe", "&amp;", "clear"} {
		i := strings.Index(source, want)
		found := false
		for _, span := range spans {
			if span.StartByte <= i && i+len(want) <= span.EndByte {
				found = true
			}
		}
		if !found {
			t.Errorf("no span covers %q: %#v", want, spans)
		}
	}
}

func TestExtractHTMLVisibleTextExcludesNonContentAndProtectsCodeAndLinks(t *testing.T) {
	source := `<head><title>hidden</title></head><p>Read <a href="/guide">the guide</a> and <code>foo-cli</code>.</p><script>hidden()</script>`
	got := ExtractHTMLVisibleText(source)
	if strings.Contains(got.Text, "hidden") || strings.Contains(got.Text, "foo-cli") {
		t.Fatalf("excluded text leaked into %q", got.Text)
	}
	if !strings.Contains(got.Text, "the guide") {
		t.Fatalf("link label missing from %q", got.Text)
	}
	doc := &Document{Format: FormatHTML, Content: source, Analysis: got.Text, Projection: got.Segments}
	linkStart := strings.Index(got.Text, "the guide")
	if !doc.IsProtectedAnalysisRange(linkStart, linkStart+len("the guide")) {
		t.Fatal("link label must be protected")
	}
	codeSeparator := strings.Index(got.Text, " and ") + len(" and ")
	if !doc.IsProtectedAnalysisRange(codeSeparator, codeSeparator+1) {
		t.Fatal("code separator must be protected")
	}
}

func TestCollectInputsDiscoversAllowedFormats(t *testing.T) {
	dir := t.TempDir()
	files := map[string]DocumentFormat{
		"page.html": FormatHTML, "fragment.htm": FormatHTML,
		"guide.md": FormatMarkdown, "notes.markdown": FormatMarkdown,
		"plain.txt": FormatText,
	}
	for name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<p>Text.</p>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"skip.js", "skip.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ignored"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	docs, err := CollectInputs([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != len(files) {
		t.Fatalf("docs = %#v", docs)
	}
	for _, doc := range docs {
		if want := files[filepath.Base(doc.Source)]; doc.Format != want {
			t.Errorf("format for %s = %q, want %q", doc.Source, doc.Format, want)
		}
	}
}

func TestHTMLDocumentDiscoveryAndVirtualChunking(t *testing.T) {
	doc, err := FromReader(strings.NewReader(`<p>First.</p><p>Second.</p>`), "page.htm", "description")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != FormatHTML || doc.AnalysisContent() != "First.\nSecond.\n" {
		t.Fatalf("document = %#v", doc)
	}
	chunks := ChunkRanges(doc, 8)
	if len(chunks) < 2 {
		t.Fatalf("chunks = %#v, want virtual chunks", chunks)
	}
	if chunks[len(chunks)-1].EndByte != len(doc.AnalysisContent()) {
		t.Fatalf("last chunk = %#v", chunks[len(chunks)-1])
	}
}

func TestHTMLProjectionRangeEdgeCases(t *testing.T) {
	source := `<p>first <em>second</em> third</p>`
	projection := ExtractHTMLVisibleText(source)
	doc := &Document{Format: FormatHTML, Content: source, Analysis: projection.Text, Projection: projection.Segments}

	second := strings.Index(projection.Text, "second")
	spans := doc.SourceSpansForAnalysisRange(second, second+len("second"))
	if len(spans) != 1 || source[spans[0].StartByte:spans[0].EndByte] != "second" {
		t.Fatalf("second spans = %#v", spans)
	}
	if got := doc.SourceSpansForAnalysisRange(-1, 1); got != nil {
		t.Fatalf("negative range = %#v", got)
	}
	if got := doc.SourceSpansForAnalysisRange(0, len(projection.Text)+1); got != nil {
		t.Fatalf("out-of-bounds range = %#v", got)
	}
	if got := doc.SourceSpansForAnalysisRange(second, second); len(got) != 0 {
		t.Fatalf("empty range = %#v", got)
	}
	if doc.IsProtectedAnalysisRange(0, len("first")) {
		t.Fatal("ordinary prose must not be protected")
	}
}

func TestExtractHTMLVisibleTextHandlesCommonEdgeCases(t *testing.T) {
	source := `<div>One<br>Two<img src="x"><script>hidden</script>&nbsp;&lt;three</div><p>Unclosed`
	got := ExtractHTMLVisibleText(source)
	for _, forbidden := range []string{"hidden", "<script"} {
		if strings.Contains(got.Text, forbidden) {
			t.Fatalf("%q leaked into %q", forbidden, got.Text)
		}
	}
	for _, expected := range []string{"One", "Two", "\u00a0", "<three", "Unclosed"} {
		if !strings.Contains(got.Text, expected) {
			t.Fatalf("%q missing from %q", expected, got.Text)
		}
	}
}

func TestAnalyzeProseHTMLUsesProjectionMap(t *testing.T) {
	source := `<p>First.</p><p>Second.</p>`
	doc, err := FromReader(strings.NewReader(source), "page.html", "description")
	if err != nil {
		t.Fatal(err)
	}
	blocks := AnalyzeProse(doc)
	if len(blocks) != 1 || blocks[0].AnalysisText != "First.\nSecond.\n" {
		t.Fatalf("blocks = %#v", blocks)
	}
	if blocks[0].StartByte != 0 || blocks[0].EndByte != len(source) {
		t.Fatalf("raw bounds = %#v", blocks[0])
	}
	if blocks[0].analysisMap[0] != strings.Index(source, "First") {
		t.Fatalf("first mapping = %d", blocks[0].analysisMap[0])
	}
}

func TestSourceSpansForAnalysisRangeUsesIdentityForText(t *testing.T) {
	doc, err := FromReader(strings.NewReader("plain text"), "notes.txt", "description")
	if err != nil {
		t.Fatal(err)
	}
	spans := doc.SourceSpansForAnalysisRange(1, 6)
	if len(spans) != 1 || spans[0] != (SourceSpan{StartByte: 1, EndByte: 6}) {
		t.Fatalf("identity spans = %#v", spans)
	}
}
