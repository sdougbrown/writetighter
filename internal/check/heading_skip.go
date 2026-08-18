package check

import (
	"fmt"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// headingSkipChecker flags heading levels that jump more than one level
// deeper than the previous heading. The Google developer documentation style
// guide maintains logical heading hierarchy: an h3 belongs under an h2, not
// directly under an h1.
type headingSkipChecker struct{}

func (headingSkipChecker) ID() string   { return "CORE.HEADING_SKIP" }
func (headingSkipChecker) Version() int { return 1 }

func (headingSkipChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, headingSkipChecker{}.ID())

	var out []report.Finding
	prevLevel := 0
	for _, block := range document.AnalyzeProse(ctx.Document) {
		if !isHeadingMarker(block.Marker) {
			continue
		}
		level := headingLevel(block.Marker)
		if prevLevel > 0 && level > prevLevel+1 {
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         headingSkipChecker{}.ID(),
				RuleVersion:    1,
				Checker:        headingSkipChecker{}.ID(),
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
				Evidence:   fmt.Sprintf("H%d follows H%d", level, prevLevel),
				Message:    "Do not skip heading levels; the hierarchy should descend one level at a time.",
				Confidence: 1,
			})
		}
		prevLevel = level
	}
	return out, nil
}

func init() { Register(headingSkipChecker{}) }
