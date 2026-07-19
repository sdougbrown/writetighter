package check

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

type denseParagraphChecker struct{}

func (denseParagraphChecker) ID() string   { return "CORE.DENSE_PARAGRAPH" }
func (denseParagraphChecker) Version() int { return 1 }
func (denseParagraphChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	var out []report.Finding
	var prose []document.Segment
	flush := func() {
		if len(prose) == 0 {
			return
		}
		text := strings.Builder{}
		for i, seg := range prose {
			if i > 0 {
				text.WriteByte(' ')
			}
			text.WriteString(seg.Text)
		}
		content := text.String()
		sentences := strings.Count(content, ".") + strings.Count(content, "!") + strings.Count(content, "?")
		words := len(strings.Fields(content))
		if sentences > 3 || words > 50 {
			first := prose[0]
			last := prose[len(prose)-1]
			path := ctx.Document.Source
			out = append(out, report.Finding{RuleID: denseParagraphChecker{}.ID(), RuleVersion: 1, Checker: denseParagraphChecker{}.ID(), CheckerVersion: 1, Enforcement: "candidate", Severity: "info", Path: &path, Range: &report.FindingRange{StartByte: first.Range.Start.Byte, EndByte: last.Range.End.Byte, StartLine: first.Range.Start.Line, StartColumn: first.Range.Start.Column, EndLine: last.Range.End.Line, EndColumn: last.Range.End.Column}, Evidence: fmt.Sprintf("%d sentences; %d words", sentences, words), Message: "Paragraph is dense.", Confidence: 1})
		}
		prose = nil
	}
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			flush()
			continue
		}
		if strings.TrimSpace(seg.Text) == "" {
			flush()
			continue
		}
		prose = append(prose, seg)
	}
	flush()
	return out, nil
}

func init() { Register(denseParagraphChecker{}) }
