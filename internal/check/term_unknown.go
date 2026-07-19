package check

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

type termUnknownChecker struct{}

func (termUnknownChecker) ID() string   { return "CORE.TERM_UNKNOWN" }
func (termUnknownChecker) Version() int { return 1 }
func (termUnknownChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.Profile == nil || ctx.Profile.Rules == nil || ctx.Profile.Rules.UnknownTermPolicy != "candidate" || ctx.Profile.Dict == nil {
		return nil, nil
	}
	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for i, word := range strings.Fields(seg.Text) {
			clean := strings.TrimFunc(word, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
			if clean == "" || ctx.Profile.Dict.Lookup(clean) != nil {
				continue
			}
			path := ctx.Document.Source
			out = append(out, report.Finding{RuleID: termUnknownChecker{}.ID(), RuleVersion: 1, Checker: termUnknownChecker{}.ID(), CheckerVersion: 1, Enforcement: "candidate", Severity: "info", Path: &path, Range: &report.FindingRange{StartByte: seg.Range.Start.Byte + i, EndByte: seg.Range.Start.Byte + i + len(word), StartLine: seg.Range.Start.Line, StartColumn: seg.Range.Start.Column + i, EndLine: seg.Range.Start.Line, EndColumn: seg.Range.Start.Column + i + len(word)}, Evidence: fmt.Sprintf("Unknown term: %s", clean), Message: fmt.Sprintf("Unknown term: %s", clean), Confidence: 1})
		}
	}
	return out, nil
}
func init() { Register(termUnknownChecker{}) }
