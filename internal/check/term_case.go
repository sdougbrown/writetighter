package check

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

type termCaseChecker struct{}

func (termCaseChecker) ID() string   { return "CORE.TERM_CASE" }
func (termCaseChecker) Version() int { return 1 }
func (termCaseChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.Profile == nil || ctx.Profile.Dict == nil {
		return nil, nil
	}
	var out []report.Finding
	for _, e := range ctx.Profile.Dict.Entries {
		if e.CanonicalCase == nil {
			continue
		}
		for _, seg := range ctx.Document.Segments {
			if seg.Type != document.SegmentProse {
				continue
			}
			text := seg.Text
			lowerText := strings.ToLower(text)
			lowerTerm := strings.ToLower(e.Term)
			idx := 0
			for {
				pos := strings.Index(lowerText[idx:], lowerTerm)
				if pos < 0 {
					break
				}
				actualStart := idx + pos
				actualEnd := actualStart + len(e.Term)
				actual := text[actualStart:actualEnd]
				if actual != *e.CanonicalCase {
					path := ctx.Document.Source
					out = append(out, report.Finding{RuleID: termCaseChecker{}.ID(), RuleVersion: 1, Checker: termCaseChecker{}.ID(), CheckerVersion: 1, Enforcement: "enforced", Severity: "warning", Path: &path, Range: &report.FindingRange{StartByte: seg.Range.Start.Byte + actualStart, EndByte: seg.Range.Start.Byte + actualEnd, StartLine: seg.Range.Start.Line, StartColumn: seg.Range.Start.Column, EndLine: seg.Range.End.Line, EndColumn: seg.Range.End.Column}, Evidence: fmt.Sprintf("canonical case is %q", *e.CanonicalCase), Message: "Term case does not match canonical form.", Confidence: 1})
				}
				idx = actualEnd
			}
		}
	}
	return out, nil
}
func init() { Register(termCaseChecker{}) }
