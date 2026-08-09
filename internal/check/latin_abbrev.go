package check

import (
	"fmt"
	"regexp"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// latinAbbrevRe matches Latin abbreviations that STE bans (GR-6):
// e.g., i.e., and etc.  These are unclear for non-native readers and
// should be written out: "for example", "that is", or name the items.
var latinAbbrevRe = regexp.MustCompile(`(?i)\b(e\.g\.|i\.e\.|etc\.?)`)

// latinAbbrevSuggestion returns replacement guidance for a Latin abbreviation,
// reading from the profile dictionary when available.
func latinAbbrevSuggestion(abbrev string, ctx *RunContext) string {
	if ctx != nil && ctx.Profile != nil && ctx.Profile.Dict != nil {
		if s, ok := ctx.Profile.Dict.BannedLatinAbbrevSuggestions[abbrev]; ok && s != "" {
			return s
		}
	}
	// Fallback for v1 profiles without latin_abbrev_suggestions.
	switch abbrev {
	case "e.g.":
		return "Write 'for example'."
	case "i.e.":
		return "Write 'that is'."
	case "etc", "etc.":
		return "Name the items or write 'and more'."
	}
	return "Replace the Latin abbreviation."
}

type latinAbbrevChecker struct{}

func (latinAbbrevChecker) ID() string   { return "CORE.LATIN_ABBREV" }
func (latinAbbrevChecker) Version() int { return 1 }

func (latinAbbrevChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, latinAbbrevChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, match := range latinAbbrevRe.FindAllStringIndex(seg.Text, -1) {
			start, end := match[0], match[1]
			word := seg.Text[start:end]
			lower := lower(word)
			msg := latinAbbrevSuggestion(lower, ctx)
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         latinAbbrevChecker{}.ID(),
				RuleVersion:    latinAbbrevChecker{}.Version(),
				Checker:        latinAbbrevChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range: &report.FindingRange{
					StartByte:   seg.Range.Start.Byte + start,
					EndByte:     seg.Range.Start.Byte + end,
					StartLine:   seg.Range.Start.Line,
					StartColumn: codePointColumn(seg.Text, start, seg.Range.Start.Column),
					EndLine:     seg.Range.Start.Line,
					EndColumn:   codePointColumn(seg.Text, end, seg.Range.Start.Column),
				},
				Evidence:   fmt.Sprintf("Latin abbreviation: %q", word),
				Message:    msg,
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(latinAbbrevChecker{}) }
