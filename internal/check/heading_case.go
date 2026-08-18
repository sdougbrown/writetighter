package check

import (
	"fmt"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// headingCaseChecker flags headings that appear to use title case. The Google
// developer documentation style guide uses sentence case for all headings and
// titles.
//
// Detection is heuristic: the heading is flagged when every content word
// after the first (a word that is not a function word) is capitalized, and
// there are at least three such words. Sentence-case headings with a run of
// proper nouns can still be flagged; that residual imprecision is acceptable
// at candidate/info enforcement, where a reviewer dismisses in one glance.
type headingCaseChecker struct{}

func (headingCaseChecker) ID() string   { return "CORE.HEADING_CASE" }
func (headingCaseChecker) Version() int { return 1 }

func (headingCaseChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, headingCaseChecker{}.ID())

	var out []report.Finding
	for _, block := range document.AnalyzeProse(ctx.Document) {
		if !isHeadingMarker(block.Marker) {
			continue
		}
		words := headingWords(block.AnalysisText)
		if len(words) < 4 {
			continue
		}
		capitalized, total := 0, 0
		for _, w := range words[1:] {
			if headingStopwords[lower(w)] {
				continue
			}
			total++
			if isCapitalized(w) {
				capitalized++
			}
		}
		if total < 3 || capitalized != total {
			continue
		}
		path := ctx.Document.Source
		out = append(out, report.Finding{
			RuleID:         headingCaseChecker{}.ID(),
			RuleVersion:    1,
			Checker:        headingCaseChecker{}.ID(),
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
			Evidence:   fmt.Sprintf("heading: %q", block.AnalysisText),
			Message:    "Heading appears to use title case; use sentence case for headings and titles.",
			Confidence: 1,
		})
	}
	return out, nil
}

func init() { Register(headingCaseChecker{}) }
