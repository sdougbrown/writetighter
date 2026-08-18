package check

import (
	"fmt"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// singleItemListChecker flags lists with exactly one item. The Google
// developer documentation style guide is explicit that a single item is not a
// list; the item should be written in prose or set off with other emphasis.
//
// Consecutive list items of the same class (bulleted or numbered) form one
// list. Continuation lines of an item belong to that item's block, so a
// multi-paragraph item is still one item.
type singleItemListChecker struct{}

func (singleItemListChecker) ID() string   { return "CORE.SINGLE_ITEM_LIST" }
func (singleItemListChecker) Version() int { return 1 }

func (singleItemListChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, singleItemListChecker{}.ID())

	blocks := document.AnalyzeProse(ctx.Document)
	var out []report.Finding
	class := -1 // -1 none, 0 bulleted, 1 numbered
	var run []document.ProseBlock

	flush := func() {
		if class >= 0 && len(run) == 1 {
			block := run[0]
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         singleItemListChecker{}.ID(),
				RuleVersion:    1,
				Checker:        singleItemListChecker{}.ID(),
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
				Evidence:   fmt.Sprintf("single-item list: %q", block.AnalysisText),
				Message:    "A single item is not a list; write it in prose.",
				Confidence: 1,
			})
		}
		run = nil
		class = -1
	}

	for _, block := range blocks {
		c := -1
		switch {
		case isUnorderedItemMarker(block.Marker):
			c = 0
		case isOrderedItemMarker(block.Marker):
			c = 1
		}
		if c != class {
			flush()
			class = c
		}
		if class >= 0 {
			run = append(run, block)
		}
	}
	flush()
	return out, nil
}

func init() { Register(singleItemListChecker{}) }
