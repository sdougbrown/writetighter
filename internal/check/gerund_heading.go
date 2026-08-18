package check

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// gerundHeadingChecker flags headings that begin with an -ing verb form. The
// Google developer documentation style guide starts task-based headings with
// a bare infinitive ("Create an instance") and conceptual headings with a
// noun phrase ("Migration to Google Cloud"); leading gerunds translate
// inconsistently and read as actions in progress.
//
// Single-word headings are exempt: some gerund nouns have no better form
// ("Billing", "Pricing").
type gerundHeadingChecker struct{}

func (gerundHeadingChecker) ID() string   { return "CORE.GERUND_HEADING" }
func (gerundHeadingChecker) Version() int { return 1 }

func (gerundHeadingChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, gerundHeadingChecker{}.ID())

	var out []report.Finding
	for _, block := range document.AnalyzeProse(ctx.Document) {
		if !isHeadingMarker(block.Marker) {
			continue
		}
		words := headingWords(block.AnalysisText)
		if len(words) < 2 {
			continue
		}
		first := lower(words[0])
		if !strings.HasSuffix(first, "ing") {
			continue
		}
		// Require at least 3 characters before the "-ing" suffix so short
		// non-gerunds ("ringing", "springing" aside, "billing" is caught) and
		// base verbs with short stems are not flagged.
		if utf8.RuneCountInString(first) < 6 {
			continue
		}
		path := ctx.Document.Source
		out = append(out, report.Finding{
			RuleID:         gerundHeadingChecker{}.ID(),
			RuleVersion:    1,
			Checker:        gerundHeadingChecker{}.ID(),
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
			Evidence:   fmt.Sprintf("heading opens with gerund: %q", words[0]),
			Message:    "Headings should start with a bare infinitive (task) or a noun phrase (concept); avoid a leading -ing form.",
			Confidence: 1,
		})
	}
	return out, nil
}

func init() { Register(gerundHeadingChecker{}) }
