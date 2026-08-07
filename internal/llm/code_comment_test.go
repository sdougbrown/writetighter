package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sdougbrown/writetighter/internal/codecomment"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
)

func codeCommentDocument(t *testing.T, source, name string) *document.Document {
	t.Helper()
	doc, err := document.FromReader(strings.NewReader(source), name, "code-comment")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func codeCommentCatalog(t *testing.T, doc *document.Document) codecomment.Catalog {
	t.Helper()
	language, ok := codecomment.DetectLanguage(doc.Source)
	if !ok {
		t.Fatalf("unsupported test filename %q", doc.Source)
	}
	catalog, err := codecomment.Extract(doc.Source, language, []byte(doc.Content))
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestReviseCodeCommentsSendsReadOnlyWholeSourceAndCatalog(t *testing.T) {
	source := "package p\r\n\t// café old\r\nfunc f() {}\r\n"
	var request Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"findings":[{"comment_id":"c0001","action":"rewrite","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"The comment is stale.","replacement":"// café current","question":null,"confidence":0.9}]}`}}}})
	}))
	defer server.Close()
	doc := codeCommentDocument(t, source, "sample.go")
	response, err := ReviseCodeComments(context.Background(), Config{BaseURL: server.URL, Model: "test", Timeout: time.Second}, doc, &profile.Resolution{})
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Messages) != 2 || !strings.Contains(request.Messages[0].Content, "untrusted, read-only") || !strings.Contains(request.Messages[0].Content, "only target authority") || !strings.Contains(request.Messages[0].Content, "confidence 0.8 or higher") || !strings.Contains(request.Messages[0].Content, "Clarifications are expected") || !strings.Contains(request.Messages[1].Content, source) || !strings.Contains(request.Messages[1].Content, `"id":"c0001"`) {
		t.Fatalf("request did not contain the complete read-only source and catalog: %#v", request.Messages)
	}
	if len(response.Revisions) != 1 {
		t.Fatalf("revisions = %#v", response)
	}
	item := response.Revisions[0]
	if item.SourceText != "// café old" || item.Range.StartByte != strings.Index(source, "// café old") || item.Range.EndByte != item.Range.StartByte+len(item.SourceText) || item.Range.StartLine != 2 || item.Range.StartColumn != 2 {
		t.Fatalf("catalog-owned CRLF/UTF-8 range was not preserved: %#v", item)
	}
}

func TestReviseCodeCommentsSkipsTransportWithoutCatalogTargets(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	doc := codeCommentDocument(t, "package p\nfunc f() {}\n", "sample.go")
	response, err := ReviseCodeComments(context.Background(), Config{BaseURL: server.URL, Model: "test", Timeout: time.Second}, doc, &profile.Resolution{})
	if err != nil {
		t.Fatal(err)
	}
	if called || len(response.Revisions) != 0 || response.Status != "ok" {
		t.Fatalf("comment-free source reached transport or returned a non-empty response: called=%t response=%#v", called, response)
	}
}

func TestValidateCodeCommentResponseRejectsUnsafeAndInvalidFindings(t *testing.T) {
	source := "package p\n// real\n\n// second\n\n// third\n\n// fourth\nvar s = \"// string\"\n"
	doc := codeCommentDocument(t, source, "sample.go")
	catalog := codeCommentCatalog(t, doc)
	raw := []byte(`{"findings":[
		{"comment_id":"unknown","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":null,"question":"What?","confidence":0.9},
		{"comment_id":"c0001","action":"rewrite","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":"// real\nx := 1","question":null,"confidence":0.9},
		{"comment_id":"c0002","action":"rewrite","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":"/* second */","question":null,"confidence":0.9},
		{"comment_id":"c0001","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":null,"question":"What?","confidence":0.9},
		{"comment_id":"c0003","action":"rewrite","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":"// third","question":"What?","confidence":0.9}
	]}`)
	response, err := validateCodeCommentResponse(raw, catalog, []byte(source), doc.Source)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 0 || response.DiscardedRewrites != 1 || response.DiscardedFindings != 5 {
		t.Fatalf("unsafe, unknown, duplicate, and low-confidence findings must be discarded: %#v", response)
	}
	if strings.Contains(responseString(response), "// string") {
		t.Fatalf("string content became a target: %#v", response)
	}
}

func TestValidateCodeCommentResponseRejectsEveryDuplicateRegardlessOfOrder(t *testing.T) {
	doc := codeCommentDocument(t, "# real\n", "sample.py")
	catalog := codeCommentCatalog(t, doc)
	valid := `{"comment_id":"c0001","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":null,"question":"What?","confidence":0.9}`
	lowConfidence := `{"comment_id":"c0001","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":null,"question":"What?","confidence":0.7}`
	for _, findings := range []string{valid + "," + lowConfidence, lowConfidence + "," + valid} {
		response, err := validateCodeCommentResponse([]byte(`{"findings":[`+findings+`]}`), catalog, []byte(doc.Content), doc.Source)
		if err != nil {
			t.Fatal(err)
		}
		if len(response.Revisions) != 0 || response.DiscardedFindings != 2 {
			t.Fatalf("duplicate target produced a finding for payload %s: %#v", findings, response)
		}
	}
}

func TestValidateCodeCommentResponseCannotTargetPythonDocstrings(t *testing.T) {
	doc := codeCommentDocument(t, "\"\"\"# not a comment\"\"\"\n# real\n", "sample.py")
	catalog := codeCommentCatalog(t, doc)
	if len(catalog.Comments) != 1 || catalog.Comments[0].Text != "# real" {
		t.Fatalf("docstring entered catalog: %#v", catalog.Comments)
	}
	response, err := validateCodeCommentResponse([]byte(`{"findings":[{"comment_id":"c0002","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":null,"question":"What?","confidence":0.9}]}`), catalog, []byte(doc.Content), doc.Source)
	if err != nil || len(response.Revisions) != 0 || response.DiscardedFindings != 1 {
		t.Fatalf("docstring-like target was not rejected: %#v, err=%v", response, err)
	}
}

func TestValidateCodeCommentResponseRejectsLowConfidence(t *testing.T) {
	doc := codeCommentDocument(t, "# real\n", "sample.py")
	catalog := codeCommentCatalog(t, doc)
	response, err := validateCodeCommentResponse([]byte(`{"findings":[{"comment_id":"c0001","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":null,"question":"What?","confidence":0.7}]}`), catalog, []byte(doc.Content), doc.Source)
	if err != nil || len(response.Revisions) != 0 || response.DiscardedRewrites != 0 || response.DiscardedFindings != 1 {
		t.Fatalf("low-confidence finding was not discarded: %#v, err=%v", response, err)
	}
}

func TestValidateCodeCommentResponseKeepsValidSiblingAndMultilineIndentation(t *testing.T) {
	source := "package p\n\t// old\n\t// rationale\nfunc f() {}\n"
	doc := codeCommentDocument(t, source, "sample.go")
	catalog := codeCommentCatalog(t, doc)
	raw := []byte(`{"findings":[
		{"comment_id":"c0001","action":"rewrite","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"Clearer rationale.","replacement":"// current\n\t// rationale","question":null,"confidence":0.9},
		{"comment_id":"unknown","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"x","replacement":null,"question":"What?","confidence":0.9}
	]}`)
	response, err := validateCodeCommentResponse(raw, catalog, []byte(source), doc.Source)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 1 || response.Revisions[0].SourceText != catalog.Comments[0].Text || response.Revisions[0].Replacement == nil || *response.Revisions[0].Replacement != "// current\n\t// rationale" || response.DiscardedRewrites != 0 || response.DiscardedFindings != 1 {
		t.Fatalf("valid multiline rewrite or discarded sibling was mishandled: %#v", response)
	}
}

func TestCodeCommentResponseRejectsMalformedTopLevel(t *testing.T) {
	doc := codeCommentDocument(t, "# real\n", "sample.py")
	catalog := codeCommentCatalog(t, doc)
	for _, raw := range [][]byte{[]byte(`{}`), []byte(`{"findings":{},"extra":true}`), []byte(`{"findings":null}`)} {
		_, err := validateCodeCommentResponse(raw, catalog, []byte(doc.Content), doc.Source)
		if err == nil {
			t.Fatalf("malformed top-level response accepted: %s", raw)
		}
	}
}

func TestReviseCodeCommentsUsesContextBudgetAboveLegacyLimit(t *testing.T) {
	source := "package p\n" + strings.Repeat("// a cataloged comment\n", 1800)
	doc := codeCommentDocument(t, source, "large.go")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"findings\":[]}"}}]}`))
	}))
	defer server.Close()
	_, err := ReviseCodeComments(context.Background(), Config{BaseURL: server.URL, Model: "test", Timeout: time.Second, ContextWindowTokens: 80000}, doc, &profile.Resolution{})
	if err != nil || !called {
		t.Fatalf("large configured whole-source request failed before transport: called=%t err=%v", called, err)
	}
}

func TestReviseCodeCommentsRejectsInsufficientContextBeforeTransport(t *testing.T) {
	source := "package p\n" + strings.Repeat("// a cataloged comment\n", 1800)
	doc := codeCommentDocument(t, source, "large.go")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	_, err := ReviseCodeComments(context.Background(), Config{BaseURL: server.URL, Model: "test", Timeout: time.Second, ContextWindowTokens: 4096}, doc, &profile.Resolution{})
	if err == nil || called || !strings.Contains(err.Error(), "--context-tokens") {
		t.Fatalf("expected actionable preflight budget error, called=%t err=%v", called, err)
	}
}

func TestCodeCommentInputLimitCapsExtremeContextWithoutOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	limit, err := codeCommentInputLimit(
		Config{Model: "test", ContextWindowTokens: maxInt},
		Request{Messages: []Message{{Role: "user", Content: "small"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if limit != maxCodeCommentInputChars {
		t.Fatalf("input limit = %d, want defensive cap %d", limit, maxCodeCommentInputChars)
	}
}

func TestCodeCommentSchemaIsStrictWhenConfigured(t *testing.T) {
	rf, err := buildCodeCommentResponseFormat("json_schema")
	if err != nil || rf == nil || rf.JSONSchema == nil || !rf.JSONSchema.Strict || !strings.Contains(string(rf.JSONSchema.Schema), `"comment_id"`) || !strings.Contains(string(rf.JSONSchema.Schema), `"maxItems": 5`) {
		t.Fatalf("code-comment response schema = %#v, err=%v", rf, err)
	}
}

func responseString(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
