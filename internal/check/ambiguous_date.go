package check

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// ambiguousDateRe matches slash-separated numeric dates (M/D/YY or YYYY). A
// date is ambiguous when the first two fields are both 12 or less: 03/04/2025
// can be read as March 4 or April 3 depending on the reader's regional
// convention. When the second field is greater than 12 it can only be a day,
// so the date is unambiguous and is not flagged.
var ambiguousDateRe = regexp.MustCompile(`\b(0?[1-9]|1[0-2])/(0?[1-9]|[12]\d|3[01])/(19|20)\d{2}\b`)

type ambiguousDateChecker struct{}

func (ambiguousDateChecker) ID() string   { return "CORE.AMBIGUOUS_DATE" }
func (ambiguousDateChecker) Version() int { return 1 }

func (ambiguousDateChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, ambiguousDateChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, m := range ambiguousDateRe.FindAllStringSubmatchIndex(seg.Text, -1) {
			// m: [full0 full1 group0 group1 group2 group3]
			fullStart, fullEnd := m[0], m[1]
			dayStart, dayEnd := m[4], m[5]
			day, err := strconv.Atoi(seg.Text[dayStart:dayEnd])
			if err != nil || day > 12 {
				continue
			}
			// A match preceded by "/" is a path segment (a URL or file path),
			// not a date.
			if fullStart > 0 && seg.Text[fullStart-1] == '/' {
				continue
			}
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         ambiguousDateChecker{}.ID(),
				RuleVersion:    1,
				Checker:        ambiguousDateChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range: &report.FindingRange{
					StartByte:   seg.Range.Start.Byte + fullStart,
					EndByte:     seg.Range.Start.Byte + fullEnd,
					StartLine:   seg.Range.Start.Line,
					StartColumn: codePointColumn(seg.Text, fullStart, seg.Range.Start.Column),
					EndLine:     seg.Range.Start.Line,
					EndColumn:   codePointColumn(seg.Text, fullEnd, seg.Range.Start.Column),
				},
				Evidence:   fmt.Sprintf("ambiguous date: %q", seg.Text[fullStart:fullEnd]),
				Message:    "Ambiguous numeric date; write it out (March 4, 2025) or in ISO 8601 form (2025-03-04).",
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(ambiguousDateChecker{}) }
