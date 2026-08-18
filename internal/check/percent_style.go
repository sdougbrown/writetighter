package check

import (
	"fmt"
	"regexp"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// percentStyleRe matches a numeral followed by the word "percent" in prose.
// The Google developer documentation style guide uses numerals and the
// percent sign without a space (40%) in all but the rare case where a
// percentage starts a sentence.
var percentStyleRe = regexp.MustCompile(`(?i)\b\d+(\.\d+)?\s+percent\b`)

type percentStyleChecker struct{}

func (percentStyleChecker) ID() string   { return "CORE.PERCENT_STYLE" }
func (percentStyleChecker) Version() int { return 1 }

func (percentStyleChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, percentStyleChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, m := range percentStyleRe.FindAllStringIndex(seg.Text, -1) {
			start, end := m[0], m[1]
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         percentStyleChecker{}.ID(),
				RuleVersion:    1,
				Checker:        percentStyleChecker{}.ID(),
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
				Evidence:   fmt.Sprintf("spelled-out percent: %q", seg.Text[start:end]),
				Message:    "Pair numerals with the percent sign (40%), not the word 'percent'.",
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(percentStyleChecker{}) }
