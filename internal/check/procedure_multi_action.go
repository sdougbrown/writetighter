package check

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

type procedureMultiActionChecker struct{}

func (procedureMultiActionChecker) ID() string   { return "CORE.PROCEDURE_MULTI_ACTION" }
func (procedureMultiActionChecker) Version() int { return 1 }
func (procedureMultiActionChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		lines := strings.Split(seg.Text, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) < 3 || trimmed[0] < '0' || trimmed[0] > '9' || !strings.Contains(trimmed, ".") {
				continue
			}
			if strings.Contains(strings.ToLower(trimmed), " and ") || strings.Contains(strings.ToLower(trimmed), " or ") {
				path := ctx.Document.Source
				out = append(out, report.Finding{RuleID: procedureMultiActionChecker{}.ID(), RuleVersion: 1, Checker: procedureMultiActionChecker{}.ID(), CheckerVersion: 1, Enforcement: "candidate", Severity: "info", Path: &path, Range: &report.FindingRange{StartByte: seg.Range.Start.Byte, EndByte: seg.Range.End.Byte, StartLine: seg.Range.Start.Line, StartColumn: seg.Range.Start.Column, EndLine: seg.Range.End.Line, EndColumn: seg.Range.End.Column}, Evidence: fmt.Sprintf("step contains multiple actions: %q", trimmed), Message: "Step may contain multiple actions.", Confidence: 1})
			}
		}
	}
	return out, nil
}
func init() { Register(procedureMultiActionChecker{}) }
