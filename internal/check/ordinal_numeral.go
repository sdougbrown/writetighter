package check

import (
	"fmt"
	"regexp"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// ordinalNumeralRe matches numerals carrying ordinal suffixes (1st, 2nd, 3rd,
// 4th, 12th) in prose. The Google developer documentation style guide
// spells out ordinal numbers in text; the suffix form is a scan artifact of
// spreadsheet-style writing.
var ordinalNumeralRe = regexp.MustCompile(`(?i)\b\d{1,3}(st|nd|rd|th)\b`)

type ordinalNumeralChecker struct{}

func (ordinalNumeralChecker) ID() string   { return "CORE.ORDINAL_NUMERAL" }
func (ordinalNumeralChecker) Version() int { return 1 }

func (ordinalNumeralChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, ordinalNumeralChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, m := range ordinalNumeralRe.FindAllStringIndex(seg.Text, -1) {
			start, end := m[0], m[1]
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         ordinalNumeralChecker{}.ID(),
				RuleVersion:    1,
				Checker:        ordinalNumeralChecker{}.ID(),
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
				Evidence:   fmt.Sprintf("ordinal numeral: %q", seg.Text[start:end]),
				Message:    "Spell out ordinal numbers in text (first, second, third).",
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(ordinalNumeralChecker{}) }
