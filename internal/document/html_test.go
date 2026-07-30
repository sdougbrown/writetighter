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

func TestCollectInputsDiscoversHTMLFiles(t *testing.T) {
	dir := t.TempDir()
	for name := range map[string]bool{"page.html": true, "fragment.htm": true} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<p>Text.</p>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	docs, err := CollectInputs([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].Format != FormatHTML || docs[1].Format != FormatHTML {
		t.Fatalf("docs = %#v", docs)
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
