package llm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
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
	// Suggestions that lose protected technical content are discarded.
	accepted := validated[:0]
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

			if validated[i].Suggestion == nil || !preservesProtectedContent(doc, r.StartByte, r.EndByte, *validated[i].Suggestion, terms) {
				continue
			}

			// Compute Unicode code-point line/column for the original document.
			r.StartLine, r.StartColumn = byteOffsetToLineColumn(doc.Content, r.StartByte)
			r.EndLine, r.EndColumn = byteOffsetToLineColumn(doc.Content, r.EndByte)
		}
		// Set path to the document source so each finding is scoped per-file.
		if validated[i].Path == nil || *validated[i].Path == "" {
			p := doc.Source
			validated[i].Path = &p
		}
		accepted = append(accepted, validated[i])
	}
	return accepted, nil
}

var protectedTokenPattern = regexp.MustCompile(`https?://[^\s<>"']+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|\b[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\b|\b(?:\d{1,3}\.){3}\d{1,3}\b|(?:\.\.?/|/)[A-Za-z0-9._~!$&()*+,;=:@%/-]+|--[A-Za-z0-9][A-Za-z0-9_-]*|\bv?\d+(?:\.\d+)*(?:[-+][A-Za-z0-9.-]+)?\b|\b[A-Za-z][A-Za-z0-9]*_[A-Za-z0-9_]+\b|\b[A-Za-z0-9]*[a-z][A-Za-z0-9]*[A-Z][A-Za-z0-9]*\b|\b[A-Z]{2,}[A-Za-z0-9]*\b`)

func preservesProtectedContent(doc *document.Document, start, end int, replacement string, terms []config.TermEntry) bool {
	if doc == nil || start < 0 || end < start || end > len(doc.Content) {
		return false
	}
	source := doc.Content[start:end]
	protected := map[string]struct{}{}
	for _, token := range protectedTokenPattern.FindAllString(source, -1) {
		token = strings.TrimRight(token, ".,;:!?)]}")
		if token != "" {
			protected[token] = struct{}{}
		}
	}
	for _, seg := range doc.Segments {
		if seg.Range.Start.Byte >= end || seg.Range.End.Byte <= start {
			continue
		}
		switch seg.Type {
		case document.SegmentInlineCode, document.SegmentLinkDest, document.SegmentInlineHTML:
			if seg.Text != "" {
				protected[seg.Text] = struct{}{}
			}
		}
	}
	for _, term := range terms {
		if term.Term != "" && strings.Contains(source, term.Term) {
			protected[term.Term] = struct{}{}
		}
	}
	for token := range protected {
		if !strings.Contains(replacement, token) {
			return false
		}
	}
	return true
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
