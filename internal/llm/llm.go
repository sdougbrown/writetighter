package llm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

// Advisor returns only advisory findings. Static findings remain owned by the caller.
func Advisor(ctx context.Context, config Config, doc *document.Document, res *profile.Resolution, findings []report.Finding, terms []config.TermEntry) ([]report.Finding, error) {
	client, err := NewClient(config)
	if err != nil {
		return nil, err
	}
	prompt, excerpt := BuildPrompt(doc, res, findings, terms)
	req := Request{Messages: []Message{{Role: "system", Content: prompt}, {Role: "user", Content: excerpt.Text}}}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("empty llm response")
	}
	active := make(map[string]struct{}, len(res.Rules.Rules))
	ruleVersions := make(map[string]int, len(res.Rules.Rules))
	for _, rule := range res.Rules.Rules {
		if rule.Enabled {
			active[rule.ID] = struct{}{}
			ruleVersions[rule.ID] = rule.Version
		}
	}
	validated, err := ValidateAdvisorResponseForRules([]byte(resp.Choices[0].Message.Content), excerpt.Text, active)
	if err != nil {
		return nil, err
	}
	// Remap advisory ranges from excerpt-relative to original-file-relative.
	// Compute authoritative original-document byte offsets plus one-based
	// Unicode code-point start/end line and column with exclusive-end semantics.
	// Also set the original path.
	for i := range validated {
		validated[i].RuleVersion = ruleVersions[validated[i].RuleID]
		validated[i].CheckerVersion = 1
		if validated[i].Range != nil {
			r := validated[i].Range

			// Reject ranges that include/cross synthetic separators or
			// noncontiguous original regions.
			if !excerpt.validExcerptRange(r.StartByte, r.EndByte) {
				return nil, fmt.Errorf("advisory finding range [%d, %d) crosses excerpt gap or noncontiguous region", r.StartByte, r.EndByte)
			}

			// Remap byte offsets to original document.
			r.StartByte = excerpt.OrigOffset(r.StartByte)
			r.EndByte = excerpt.OrigOffset(r.EndByte)

			// Compute Unicode code-point line/column for the original document.
			r.StartLine, r.StartColumn = byteOffsetToLineColumn(doc.Content, r.StartByte)
			r.EndLine, r.EndColumn = byteOffsetToLineColumn(doc.Content, r.EndByte)
		}
		// Set path to the document source so each finding is scoped per-file.
		if validated[i].Path == nil || *validated[i].Path == "" {
			p := doc.Source
			validated[i].Path = &p
		}
	}
	return validated, nil
}

// byteOffsetToLineColumn converts a zero-based byte offset into one-based
// line number and Unicode code-point column number (exclusive end).
// If byteOffset is at len(content), it returns the position of the
// (nonexistent) character after the last one.
func byteOffsetToLineColumn(content string, byteOffset int) (line, col int) {
	if byteOffset <= 0 {
		return 1, 1
	}
	if byteOffset > len(content) {
		byteOffset = len(content)
	}
	line = 1
	col = 1
	bytes := 0
	for bytes < byteOffset {
		r, size := utf8.DecodeRuneInString(content[bytes:])
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		bytes += size
	}
	return
}

func Host(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return base
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}
