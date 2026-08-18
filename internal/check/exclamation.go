package check

import (
	"fmt"
	"regexp"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// exclamationChecker flags exclamation points in prose. The Google developer
// documentation style guide directs authors to avoid exclamation marks: they
// signal enthusiasm, not information, and read as unprofessional in technical
// documentation. Prose segments exclude inline code and code blocks, so "!"
// in shell prompts, code, or link destinations is not flagged.
var exclamationRe = regexp.MustCompile(`!`)

type exclamationChecker struct{}

func (exclamationChecker) ID() string   { return "CORE.EXCLAMATION" }
func (exclamationChecker) Version() int { return 1 }

func (exclamationChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, exclamationChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, m := range exclamationRe.FindAllStringIndex(seg.Text, -1) {
			start, end := m[0], m[1]
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         exclamationChecker{}.ID(),
				RuleVersion:    1,
				Checker:        exclamationChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range: &report.FindingRange{
					StartByte:   seg.Range.Start.Byte + start,
					EndByte:     seg.Range.Start.Byte + end,
					StartLine:   seg.Range.Start.Line,
					StartColumn: codePointColumn(seg.Text, start, seg.Range.Start.Column),
					EndLine:     seg.Range.Start.Line,
					EndColumn:   codePointColumn(seg.Text, end, seg.Range.Start.Column),
				},
				Evidence:   fmt.Sprintf("exclamation point at %d:%d", seg.Range.Start.Line, codePointColumn(seg.Text, start, seg.Range.Start.Column)),
				Message:    "Avoid exclamation points in technical documentation.",
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(exclamationChecker{}) }
