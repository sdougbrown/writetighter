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
