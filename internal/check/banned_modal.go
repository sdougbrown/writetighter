package check

import (
	"fmt"
	"regexp"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// bannedModalRe matches STE-unapproved modal verbs: should, would, may,
// might, could. These hedge or hypothesize where technical documentation
// should state facts, requirements, or possibilities directly.
// Approved modals (can, will, must) are not matched.
var bannedModalRe = regexp.MustCompile(`(?i)\b(should|would|may|might|could)\b`)

// bannedModalSuggestion returns replacement guidance for a banned modal,
// reading from the profile dictionary when available. The embedded profile
// always provides the suggestions map, so the fallback here is only reached
// in edge cases (nil dict) and is intentionally generic.
func bannedModalSuggestion(modal string, ctx *RunContext) string {
	if ctx != nil && ctx.Profile != nil && ctx.Profile.Dict != nil {
		if s, ok := ctx.Profile.Dict.BannedModalSuggestions[modal]; ok && s != "" {
			return s
		}
	}
	return "Replace the unapproved modal."
}

type bannedModalChecker struct{}

func (bannedModalChecker) ID() string   { return "CORE.BANNED_MODAL" }
func (bannedModalChecker) Version() int { return 1 }

func (bannedModalChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, bannedModalChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, match := range bannedModalRe.FindAllStringIndex(seg.Text, -1) {
			start, end := match[0], match[1]
			word := seg.Text[start:end]
			lower := lower(word)
			msg := bannedModalSuggestion(lower, ctx)
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         bannedModalChecker{}.ID(),
				RuleVersion:    bannedModalChecker{}.Version(),
				Checker:        bannedModalChecker{}.ID(),
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
				Evidence:   fmt.Sprintf("banned modal: %q", word),
				Message:    msg,
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(bannedModalChecker{}) }
