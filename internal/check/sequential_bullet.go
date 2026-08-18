package check

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// sequentialWords open a step of a sequence. A bulleted item that begins with
// one of them is describing an order of operations, which the Google
// developer documentation style guide assigns to numbered lists.
var sequentialWords = map[string]bool{
	"first": true, "second": true, "third": true, "fourth": true,
	"fifth": true, "sixth": true, "seventh": true, "eighth": true,
	"ninth": true, "tenth": true, "then": true, "next": true,
	"finally": true, "lastly": true,
}

// sequentialBulletChecker flags bulleted (unnumbered) list items that open
// with a sequencing word, indicating the list is really a sequence and should
// be numbered.
type sequentialBulletChecker struct{}

func (sequentialBulletChecker) ID() string   { return "CORE.SEQUENTIAL_BULLET" }
func (sequentialBulletChecker) Version() int { return 1 }

func (sequentialBulletChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, sequentialBulletChecker{}.ID())

	var out []report.Finding
	for _, block := range document.AnalyzeProse(ctx.Document) {
		if !isUnorderedItemMarker(block.Marker) {
			continue
		}
		words := headingWords(block.AnalysisText)
		if len(words) == 0 {
			continue
		}
		first := strings.Trim(lower(words[0]), ".,:;")
		if !sequentialWords[first] {
			continue
		}
		path := ctx.Document.Source
		out = append(out, report.Finding{
			RuleID:         sequentialBulletChecker{}.ID(),
			RuleVersion:    1,
			Checker:        sequentialBulletChecker{}.ID(),
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
			Evidence:   fmt.Sprintf("bulleted item opens with %q", words[0]),
			Message:    "Sequence words indicate ordered steps; use a numbered list for sequences.",
			Confidence: 1,
		})
	}
	return out, nil
}

func init() { Register(sequentialBulletChecker{}) }
