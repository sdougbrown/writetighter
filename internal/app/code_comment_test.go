package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

func TestUsesCodeCommentProtocolOnlyForSupportedFileRevision(t *testing.T) {
	goDoc, err := document.FromReader(strings.NewReader("package p\n// real\n"), "sample.go", "code-comment")
	if err != nil {
		t.Fatal(err)
	}
	txtDoc, err := document.FromReader(strings.NewReader("// prose style\n"), "sample.txt", "code-comment")
	if err != nil {
		t.Fatal(err)
	}
	params := ReviseParams{Paths: []string{"sample.go"}, Kind: "code-comment"}
	if !usesCodeCommentProtocol(params, goDoc) {
		t.Fatal("supported code file did not select the ID protocol")
	}
	if usesCodeCommentProtocol(params, txtDoc) {
		t.Fatal("unsupported extension selected the ID protocol")
	}
	params.Stdin = true
	if usesCodeCommentProtocol(params, goDoc) {
		t.Fatal("stdin selected the ID protocol")
	}
	params.Stdin = false
	text := "// direct text"
	params.Text = &text
	if usesCodeCommentProtocol(params, goDoc) {
		t.Fatal("--text selected the ID protocol")
	}
}

func TestRunReviseRoutesSupportedCodeFilesToCatalogProtocol(t *testing.T) {
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"findings":[{"comment_id":"c0001","action":"clarification","principle_ids":["CORE.EXPLICIT_RELATIONSHIPS"],"reason":"Missing context.","replacement":null,"question":"What does this protect?","confidence":0.9}]}`}}}})
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte("package p\n// real\nfunc f() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Paths: []string{path}, Kind: "code-comment", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, "complete editable-comment catalog") || strings.Contains(request.Messages[1].Content, "<revise-text>") {
		t.Fatalf("supported file did not use catalog request: %#v", request.Messages)
	}
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Revisions) != 1 || response.Revisions[0].SourceText != "// real" || response.Revisions[0].Range.StartByte != strings.Index("package p\n// real", "// real") {
		t.Fatalf("report was not catalog-owned: %#v", response)
	}
	human, err := report.RenderReviseHuman(&response)
	if err != nil || !strings.Contains(human, `source: "// real"`) {
		t.Fatalf("human report lost catalog-owned source text: %q, err=%v", human, err)
	}
}

func TestRunReviseSkipsModelForSupportedFileWithoutComments(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	source := "package p\nfunc f() {}\n"
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Paths: []string{path}, Kind: "code-comment", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	if called {
		t.Fatal("comment-free source reached the model endpoint")
	}
	var response report.ReviseResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Analysis) != 1 || response.Analysis[0].ModelRequests != 0 || response.Analysis[0].Chunks != 0 || response.Analysis[0].AnalyzedBytes != len(source) || !response.Analysis[0].Complete {
		t.Fatalf("comment-free analysis metadata = %#v", response.Analysis)
	}
}

func TestRunReviseRejectsReferencesForCodeAwareFiles(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "sample.go")
	referencePath := filepath.Join(dir, "reference.md")
	if err := os.WriteFile(sourcePath, []byte("package p\n// real\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(referencePath, []byte("Repository context.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := (&App{}).RunRevise(ReviseParams{
		Paths:          []string{sourcePath},
		Kind:           "code-comment",
		Format:         "json",
		ReferencePaths: []string{referencePath},
	})
	if err == nil || !strings.Contains(err.Error(), "does not support --reference") {
		t.Fatalf("expected explicit code-aware reference error, got %v", err)
	}
	if called {
		t.Fatal("reference incompatibility reached the model endpoint")
	}
}

func TestRunReviseKeepsUnsupportedAndStdinCodeCommentsOnLegacyPath(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request.Messages[1].Content)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"findings\":[]}"}}]}`))
	}))
	defer server.Close()
	writeReviseUserConfig(t, server.URL, "")
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(path, []byte("// prose fallback\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&App{}).RunRevise(ReviseParams{Paths: []string{path}, Kind: "code-comment", Format: "json"}); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("// stdin fallback\n"))
	_ = w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	output := captureStdout(t, func() {
		if err := (&App{}).RunRevise(ReviseParams{Stdin: true, Kind: "code-comment", Format: "json"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(requests) != 2 || !strings.Contains(requests[0], "<revise-text>") || !strings.Contains(requests[1], "<revise-text>") {
		t.Fatalf("unsupported or stdin input did not retain legacy prompt: %#v", requests)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"status": "ok"`)) {
		t.Fatalf("stdin fallback did not render a public revise report: %s", output.String())
	}
}
