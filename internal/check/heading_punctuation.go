package check

import (
	"fmt"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// headingPunctuationChecker flags headings that end with a period. The Google
// developer documentation style guide keeps heading punctuation simple: a
// trailing period is a sign the heading is a sentence, not a label, and the
// heading should be rewritten.
type headingPunctuationChecker struct{}

func (headingPunctuationChecker) ID() string   { return "CORE.HEADING_PUNCTUATION" }
func (headingPunctuationChecker) Version() int { return 1 }

func (headingPunctuationChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, headingPunctuationChecker{}.ID())

	var out []report.Finding
	for _, block := range document.AnalyzeProse(ctx.Document) {
		if !isHeadingMarker(block.Marker) {
			continue
		}
		runes := []rune(block.AnalysisText)
		if len(runes) == 0 || runes[len(runes)-1] != '.' {
			continue
		}
		// An ellipsis is not a trailing period.
		if len(runes) > 1 && runes[len(runes)-2] == '.' {
			continue
		}
		path := ctx.Document.Source
		out = append(out, report.Finding{
			RuleID:         headingPunctuationChecker{}.ID(),
			RuleVersion:    1,
			Checker:        headingPunctuationChecker{}.ID(),
			CheckerVersion: 1,
			Enforcement:    enforcement,
			Severity:       severity,
			Path:           &path,
			Range: &report.FindingRange{
				StartByte:   block.StartByte,
				EndByte:     block.EndByte,
				StartLine:   block.StartLine,
				StartColumn: block.StartColumn,
				EndLine:     block.EndLine,
				EndColumn:   block.EndColumn,
			},
			Evidence:   fmt.Sprintf("heading ends with period: %q", block.AnalysisText),
			Message:    "Headings should not end with a period; rewrite the heading as a label.",
			Confidence: 1,
		})
	}
	return out, nil
}

func init() { Register(headingPunctuationChecker{}) }
