package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sdougbrown/writetighter/internal/document"
)

func TestReviseHTMLDeclaresVirtualCoordinatesAndRawSpans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"findings":[{"kind":"clarification","source_text":"One & two.","source_range":{"start":0,"end":10},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"Needs context.","replacement":null,"question":"Which two values are meant?","confidence":0.8}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}},
		})
	}))
	defer srv.Close()

	doc, err := document.FromReader(strings.NewReader(`<p>One &amp; <em>two</em>.</p>`), "page.html", "description")
	if err != nil {
		t.Fatal(err)
	}
	response, err := Revise(context.Background(), Config{BaseURL: srv.URL, Model: "test", Timeout: time.Second}, doc, testProfile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 1 {
		t.Fatalf("revisions = %#v", response.Revisions)
	}
	revision := response.Revisions[0]
	if revision.SourceFormat != "html" || revision.RangeBasis != "visible_text" {
		t.Fatalf("coordinate declaration = %#v", revision)
	}
	if revision.Range.StartByte != 0 || revision.Range.EndByte != len("One & two.") {
		t.Fatalf("range must remain virtual: %#v", revision.Range)
	}
	if len(revision.SourceSpans) < 3 {
		t.Fatalf("source spans = %#v, want split raw provenance", revision.SourceSpans)
	}
	if revision.SourceSpans[0].StartByte != strings.Index(doc.Content, "One") {
		t.Fatalf("first raw span = %#v", revision.SourceSpans[0])
	}
}

func TestReviseHTMLRejectsRewriteTouchingProtectedLinkText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"findings":[{"kind":"rewrite","source_text":"guide","source_range":{"start":5,"end":10},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"Shorter.","replacement":"docs","question":null,"confidence":0.8}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}},
		})
	}))
	defer srv.Close()

	doc, err := document.FromReader(strings.NewReader(`<p>Read <a href="/docs">guide</a>.</p>`), "page.html", "description")
	if err != nil {
		t.Fatal(err)
	}
	response, err := Revise(context.Background(), Config{BaseURL: srv.URL, Model: "test", Timeout: time.Second}, doc, testProfile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 0 || response.DiscardedRewrites != 1 {
		t.Fatalf("protected rewrite must be discarded: %#v", response)
	}
}

func TestReviseChunkHTMLUsesVirtualOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"findings":[{"kind":"clarification","source_text":"Second.","source_range":{"start":0,"end":7},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"Context.","replacement":null,"question":"Which second value?","confidence":0.8}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer srv.Close()
	doc, err := document.FromReader(strings.NewReader(`<p>First.</p><p>Second.</p>`), "page.html", "description")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(doc.AnalysisContent(), "Second.")
	response, err := ReviseChunk(context.Background(), Config{BaseURL: srv.URL, Model: "test", Timeout: time.Second}, doc, testProfile(), nil, nil, start, start+len("Second."))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 1 || response.Revisions[0].Range.StartByte != start || response.Revisions[0].RangeBasis != "visible_text" {
		t.Fatalf("chunk response = %#v", response)
	}
}

func TestBuildReviseExcerptTruncatesHTMLVirtualText(t *testing.T) {
	doc, err := document.FromReader(strings.NewReader(`<p>`+strings.Repeat("word ", MaxInputChars)+`</p>`), "page.html", "description")
	if err != nil {
		t.Fatal(err)
	}
	excerpt := buildReviseExcerpt(doc)
	if !excerpt.virtual || len(excerpt.Text) != MaxInputChars || !strings.HasPrefix(doc.AnalysisContent(), excerpt.Text) {
		t.Fatalf("excerpt = %#v", excerpt)
	}
}
