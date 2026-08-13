package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RewriteResponse is the top-level structured JSON response for
// `writetighter rewrite`.
type RewriteResponse struct {
	SchemaVersion  int    `json:"schema_version"`
	ToolVersion    string `json:"tool_version"`
	Status         string `json:"status"`
	ProfileID      string `json:"profile_id"`
	ProfileVersion string `json:"profile_version"`
	LLMModel       string `json:"llm_model"`
	LLMProvider    string `json:"llm_provider"`
	SourcePath     string `json:"source_path"`
	InputBytes     int    `json:"input_bytes"`
	OutputBytes    int    `json:"output_bytes"`
	RewrittenText  string `json:"rewritten_text"`
	Discarded      bool   `json:"discarded"`
	DiscardReason  string `json:"discard_reason,omitempty"`
	LintFindings   int    `json:"lint_findings"`
}

// RenderRewriteJSON renders a RewriteResponse as indented JSON.
func RenderRewriteJSON(r *RewriteResponse) (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// RenderRewriteHuman renders a RewriteResponse as human-readable text.
func RenderRewriteHuman(r *RewriteResponse) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	if r.ProfileID != "" {
		fmt.Fprintf(&b, "profile: %s@%s\n", r.ProfileID, r.ProfileVersion)
	}
	if r.LLMModel != "" {
		fmt.Fprintf(&b, "model: %s\n", r.LLMModel)
	}
	fmt.Fprintf(&b, "source: %s\n", r.SourcePath)
	fmt.Fprintf(&b, "input: %d bytes → output: %d bytes\n", r.InputBytes, r.OutputBytes)
	if r.Discarded {
		switch r.DiscardReason {
		case "injected_content":
			fmt.Fprintf(&b, "discarded: rewrite introduced new security-relevant content (URL, email, or IP) not present in the source; original returned\n")
		default:
			fmt.Fprintf(&b, "discarded: rewrite failed protected-content validation; original returned\n")
		}
	}
	if r.LintFindings > 0 {
		fmt.Fprintf(&b, "lint findings passed as context: %d\n", r.LintFindings)
	}
	fmt.Fprintf(&b, "---\n")
	b.WriteString(r.RewrittenText)
	if !strings.HasSuffix(r.RewrittenText, "\n") {
		b.WriteString("\n")
	}
	return b.String(), nil
}
