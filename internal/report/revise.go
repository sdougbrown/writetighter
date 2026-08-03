package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReferenceContext reports metadata about reference material sent with the request.
type ReferenceContext struct {
	Paths               []string `json:"paths"`
	Files               []string `json:"files"`
	InputBytes          int      `json:"input_bytes"`
	IncludedBytes       int      `json:"included_bytes"`
	Complete            bool     `json:"complete"`
	ContextWindowTokens int      `json:"context_window_tokens"`
	MaxOutputTokens     int      `json:"max_output_tokens"`
}

// ReviseResponse is the top-level structured JSON response for `writetighter revise`.
type ReviseResponse struct {
	SchemaVersion     int               `json:"schema_version"`
	ToolVersion       string            `json:"tool_version"`
	Status            string            `json:"status"`
	ProfileID         string            `json:"profile_id"`
	ProfileVersion    string            `json:"profile_version"`
	LLMModel          string            `json:"llm_model"`
	LLMProvider       string            `json:"llm_provider"`
	Sources           []string          `json:"sources"`
	Analysis          []SourceAnalysis  `json:"analysis"`
	DiscardedRewrites int               `json:"discarded_rewrites"`
	Errors            []RevisionError   `json:"errors,omitempty"`
	Revisions         []RevisionItem    `json:"revisions"`
	ReferenceContext  *ReferenceContext `json:"reference_context,omitempty"`
}

// SourceAnalysis reports revision coverage for one input document.
type SourceAnalysis struct {
	SourcePath    string `json:"source_path"`
	InputBytes    int    `json:"input_bytes"` // original source bytes
	AnalyzedBytes int    `json:"analyzed_bytes"`
	Chunks        int    `json:"chunks"`
	ModelRequests int    `json:"model_requests"`
	Complete      bool   `json:"complete"`
	SourceFormat  string `json:"source_format,omitempty"`
	RangeBasis    string `json:"range_basis,omitempty"`
}

// RevisionError identifies a document whose required model call or response failed.
type RevisionError struct {
	SourcePath string `json:"source_path"`
	Message    string `json:"message"`
}

// RevisionItem is a single revision suggestion from `writetighter revise`.
type RevisionItem struct {
	Kind         string      `json:"kind"` // "rewrite" or "clarification"
	SourcePath   string      `json:"source_path"`
	SourceText   string      `json:"source_text"`
	Range        ReviseRange `json:"range"`
	PrincipleIDs []string    `json:"principle_ids"`
	Reason       string      `json:"reason"`
	Replacement  *string     `json:"replacement,omitempty"` // rewrite only
	Question     *string     `json:"question,omitempty"`    // clarification only
	Confidence   float64     `json:"confidence"`
	// SourceFormat and RangeBasis identify Range coordinates.
	// HTML ranges use visible_text and include raw source spans.
	SourceFormat string       `json:"source_format,omitempty"`
	RangeBasis   string       `json:"range_basis,omitempty"`
	SourceSpans  []SourceSpan `json:"source_spans,omitempty"`
}

// SourceSpan is a half-open byte range in the original input document.
type SourceSpan struct {
	StartByte int `json:"start_byte"`
	EndByte   int `json:"end_byte"`
}

// ReviseRange describes the source range of a revision suggestion.
type ReviseRange struct {
	StartByte   int `json:"start_byte"`
	EndByte     int `json:"end_byte"`
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

// RenderReviseJSON renders a ReviseResponse as indented JSON.
func RenderReviseJSON(r *ReviseResponse) (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// RenderReviseHuman renders a ReviseResponse as human-readable text.
func RenderReviseHuman(r *ReviseResponse) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	if r.ProfileID != "" {
		fmt.Fprintf(&b, "profile: %s@%s\n", r.ProfileID, r.ProfileVersion)
	}
	if r.LLMModel != "" {
		fmt.Fprintf(&b, "model: %s\n", r.LLMModel)
	}
	for _, item := range r.Analysis {
		fmt.Fprintf(&b, "analysis: %s chunks=%d requests=%d bytes=%d/%d complete=%t\n", item.SourcePath, item.Chunks, item.ModelRequests, item.AnalyzedBytes, item.InputBytes, item.Complete)
	}
	if r.ReferenceContext != nil {
		fmt.Fprintf(&b, "reference context: %d files, %d/%d bytes, complete=%t\n",
			len(r.ReferenceContext.Files), r.ReferenceContext.IncludedBytes,
			r.ReferenceContext.InputBytes, r.ReferenceContext.Complete)
	}
	if r.DiscardedRewrites > 0 {
		fmt.Fprintf(&b, "discarded unsafe rewrites: %d\n", r.DiscardedRewrites)
	}
	for _, item := range r.Errors {
		fmt.Fprintf(&b, "error: %s: %s\n", item.SourcePath, item.Message)
	}
	if len(r.Revisions) == 0 {
		return b.String(), nil
	}
	for i, rev := range r.Revisions {
		if i > 0 {
			b.WriteString("---\n")
		}
		fmt.Fprintf(&b, "kind: %s\n", rev.Kind)
		fmt.Fprintf(&b, "path: %s\n", rev.SourcePath)
		fmt.Fprintf(&b, "source: %q\n", rev.SourceText)
		fmt.Fprintf(&b, "range: [%d:%d - %d:%d] bytes %d-%d\n",
			rev.Range.StartLine, rev.Range.StartColumn,
			rev.Range.EndLine, rev.Range.EndColumn,
			rev.Range.StartByte, rev.Range.EndByte)
		if rev.RangeBasis != "" {
			fmt.Fprintf(&b, "range basis: %s\n", rev.RangeBasis)
		}
		if len(rev.SourceSpans) > 0 {
			fmt.Fprintf(&b, "source spans: %v\n", rev.SourceSpans)
		}
		fmt.Fprintf(&b, "principles: %s\n", strings.Join(rev.PrincipleIDs, ", "))
		fmt.Fprintf(&b, "reason: %s\n", rev.Reason)
		fmt.Fprintf(&b, "confidence: %.2f\n", rev.Confidence)
		if rev.Kind == "rewrite" && rev.Replacement != nil {
			fmt.Fprintf(&b, "replacement: %s\n", *rev.Replacement)
		}
		if rev.Kind == "clarification" && rev.Question != nil {
			fmt.Fprintf(&b, "question: %s\n", *rev.Question)
		}
	}
	return b.String(), nil
}
