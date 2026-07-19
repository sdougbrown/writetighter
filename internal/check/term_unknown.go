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
		text := seg.Text
		pos := 0
		for _, word := range strings.Fields(text) {
			wordIdx := strings.Index(text[pos:], word)
			if wordIdx < 0 {
				continue
			}
			wordByteStart := pos + wordIdx
			wordByteEnd := wordByteStart + len(word)

			clean := strings.TrimFunc(word, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
			if clean == "" || ctx.Profile.Dict.Lookup(clean) != nil {
				pos = wordByteEnd
				continue
			}
			path := ctx.Document.Source
			out = append(out, report.Finding{RuleID: termUnknownChecker{}.ID(), RuleVersion: 1, Checker: termUnknownChecker{}.ID(), CheckerVersion: 1, Enforcement: "candidate", Severity: "info", Path: &path, Range: &report.FindingRange{StartByte: seg.Range.Start.Byte + wordByteStart, EndByte: seg.Range.Start.Byte + wordByteEnd, StartLine: seg.Range.Start.Line, StartColumn: seg.Range.Start.Column, EndLine: seg.Range.End.Line, EndColumn: seg.Range.End.Column}, Evidence: fmt.Sprintf("Unknown term: %s", clean), Message: fmt.Sprintf("Unknown term: %s", clean), Confidence: 1})
			pos = wordByteEnd
		}
	}
	return out, nil
}
func init() { Register(termUnknownChecker{}) }
