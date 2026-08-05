package check

import (
	"fmt"
	"regexp"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// contractionRe matches common English contractions in prose.
// It catches suffix contractions (n't, 'll, 're, 've, 'd) and a specific
// set of 's contractions (it's, that's, there's, here's, what's, who's, let's)
// that are unambiguously contractions rather than possessives.
var contractionRe = regexp.MustCompile(`(?i)\b\w+(n't|'ll|'re|'ve|'d)\b|\b(it's|that's|there's|here's|what's|who's|let's)\b`)

type contractionChecker struct{}

func (contractionChecker) ID() string   { return "CORE.CONTRACTION" }
func (contractionChecker) Version() int { return 1 }

func (contractionChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, contractionChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, match := range contractionRe.FindAllStringIndex(seg.Text, -1) {
			start, end := match[0], match[1]
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         contractionChecker{}.ID(),
				RuleVersion:    contractionChecker{}.Version(),
				Checker:        contractionChecker{}.ID(),
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
				Evidence:   fmt.Sprintf("contraction: %q", seg.Text[start:end]),
				Message:    "Expand the contraction. Write the words in full.",
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(contractionChecker{}) }
