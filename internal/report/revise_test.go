package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReviseResponseJSON(t *testing.T) {
	replacement := "short text."
	resp := &ReviseResponse{
		SchemaVersion: 1,
		ToolVersion:   "0.1.0",
		Status:        "ok",
		Revisions: []RevisionItem{
			{
				Kind:       "rewrite",
				SourcePath: "test.md",
				SourceText: "long text.",
				Range: ReviseRange{
					StartByte: 0, EndByte: 10,
					StartLine: 1, StartColumn: 1,
					EndLine: 1, EndColumn: 11,
				},
				PrincipleIDs: []string{"CORE.SENTENCE_LENGTH"},
				Reason:       "Sentence too long.",
				Replacement:  &replacement,
				Confidence:   0.85,
				SourceFormat: "html",
				RangeBasis:   "visible_text",
				SourceSpans:  []SourceSpan{{StartByte: 4, EndByte: 14}},
			},
		},
	}

	got, err := RenderReviseJSON(resp)
	if err != nil {
		t.Fatal(err)
	}

	var parsed ReviseResponse
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	if len(parsed.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(parsed.Revisions))
	}
	if parsed.Revisions[0].Kind != "rewrite" {
		t.Fatalf("expected kind rewrite, got %q", parsed.Revisions[0].Kind)
	}
	if parsed.Revisions[0].Range.StartByte != 0 || parsed.Revisions[0].Range.EndByte != 10 {
		t.Fatalf("bad range: %+v", parsed.Revisions[0].Range)
	}
	if *parsed.Revisions[0].Replacement != replacement {
		t.Fatalf("bad replacement: %q", *parsed.Revisions[0].Replacement)
	}
	if parsed.Revisions[0].SourceFormat != "html" || parsed.Revisions[0].RangeBasis != "visible_text" || len(parsed.Revisions[0].SourceSpans) != 1 {
		t.Fatalf("missing HTML metadata: %#v", parsed.Revisions[0])
	}
}

func TestReviseResponseHuman(t *testing.T) {
	replacement := "short text."
	resp := &ReviseResponse{
		SchemaVersion: 1,
		ToolVersion:   "0.1.0",
		Status:        "ok",
		Revisions: []RevisionItem{
			{
				Kind:       "rewrite",
				SourcePath: "test.md",
				SourceText: "long text.",
				Range: ReviseRange{
					StartByte: 0, EndByte: 10,
					StartLine: 1, StartColumn: 1,
					EndLine: 1, EndColumn: 11,
				},
				PrincipleIDs: []string{"CORE.SENTENCE_LENGTH"},
				Reason:       "Sentence too long.",
				Replacement:  &replacement,
				Confidence:   0.85,
				SourceFormat: "html",
				RangeBasis:   "visible_text",
				SourceSpans:  []SourceSpan{{StartByte: 4, EndByte: 14}},
			},
		},
	}

	got, err := RenderReviseHuman(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "rewrite") {
		t.Fatal("expected kind in human output")
	}
	if !strings.Contains(got, "short text.") {
		t.Fatal("expected replacement in human output")
	}
	if !strings.Contains(got, "test.md") {
		t.Fatal("expected path in human output")
	}
	if !strings.Contains(got, "range basis: visible_text") || !strings.Contains(got, "source spans:") {
		t.Fatalf("expected HTML metadata in human output: %s", got)
	}
}

func TestReviseResponseIncludesDiscardAndErrorMetadata(t *testing.T) {
	resp := &ReviseResponse{
		SchemaVersion:     1,
		Status:            "partial",
		Analysis:          []SourceAnalysis{{SourcePath: "good.md", InputBytes: 100, AnalyzedBytes: 80, Chunks: 2, Complete: false}},
		DiscardedRewrites: 2,
		Errors:            []RevisionError{{SourcePath: "bad.md", Message: "invalid model response"}},
		Revisions:         []RevisionItem{},
	}
	jsonOutput, err := RenderReviseJSON(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jsonOutput, "discarded_rewrites") || !strings.Contains(jsonOutput, "bad.md") {
		t.Fatalf("missing structured metadata: %s", jsonOutput)
	}
	humanOutput, err := RenderReviseHuman(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(humanOutput, "bytes=80/100 complete=false") || !strings.Contains(humanOutput, "discarded unsafe rewrites: 2") || !strings.Contains(humanOutput, "invalid model response") {
		t.Fatalf("missing human metadata: %s", humanOutput)
	}
}

func TestReviseResponseHumanEmpty(t *testing.T) {
	resp := &ReviseResponse{
		SchemaVersion: 1,
		ToolVersion:   "0.1.0",
		Status:        "ok",
		Revisions:     nil,
	}
	got, err := RenderReviseHuman(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "status: ok") {
		t.Fatalf("expected status in output, got: %s", got)
	}
}

func TestReviseResponseClarification(t *testing.T) {
	question := "Did you mean 'optimize'?"
	resp := &ReviseResponse{
		SchemaVersion: 1,
		ToolVersion:   "0.1.0",
		Status:        "ok",
		Revisions: []RevisionItem{
			{
				Kind:         "clarification",
				SourcePath:   "test.md",
				SourceText:   "term?",
				Range:        ReviseRange{StartByte: 0, EndByte: 5},
				PrincipleIDs: []string{"CORE.TERM_UNKNOWN"},
				Reason:       "Term is unclear.",
				Question:     &question,
				Confidence:   0.72,
			},
		},
	}

	got, err := RenderReviseJSON(resp)
	if err != nil {
		t.Fatal(err)
	}

	var parsed ReviseResponse
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Revisions[0].Kind != "clarification" {
		t.Fatalf("expected kind clarification, got %q", parsed.Revisions[0].Kind)
	}
	if *parsed.Revisions[0].Question != question {
		t.Fatalf("bad question: %q", *parsed.Revisions[0].Question)
	}
	if parsed.Revisions[0].Replacement != nil {
		t.Fatal("clarification should not have replacement")
	}
}
