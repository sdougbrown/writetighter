package check

import (
	"fmt"
	"regexp"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// bannedModalRe matches STE-unapproved modal verbs: should, would, may,
// might, could.  These hedge or hypothesize where technical documentation
// should state facts, requirements, or possibilities directly.
// Approved modals (can, will, must) are not matched.
var bannedModalRe = regexp.MustCompile(`(?i)\b(should|would|may|might|could)\b`)

// bannedModalSuggestions maps each banned modal to its fix guidance.
var bannedModalSuggestions = map[string]string{
	"should": "Write 'must' if required, or delete if optional.",
	"would":  "Restructure: state the fact or write 'If X occurs, Y occurs.'",
	"may":    "Write 'can' for possibility or permission.",
	"might":  "Write 'can' for possibility.",
	"could":  "Write 'can' for possibility.",
}

type bannedModalChecker struct{}

func (bannedModalChecker) ID() string   { return "CORE.BANNED_MODAL" }
func (bannedModalChecker) Version() int { return 1 }

func (bannedModalChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := "candidate", "info"
	if ctx.Profile != nil && ctx.Profile.Rules != nil {
		for _, rule := range ctx.Profile.Rules.Rules {
			if rule.ID != (bannedModalChecker{}).ID() {
				continue
			}
			if rule.Enforcement != "" {
				enforcement = rule.Enforcement
			}
			if rule.Severity != "" {
				severity = rule.Severity
			}
			break
		}
	}

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, match := range bannedModalRe.FindAllStringIndex(seg.Text, -1) {
			start, end := match[0], match[1]
			word := seg.Text[start:end]
			lower := lower(word)
			msg := "Replace the unapproved modal."
			if suggestion, ok := bannedModalSuggestions[lower]; ok {
				msg = suggestion
			}
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         bannedModalChecker{}.ID(),
				RuleVersion:    1,
				Checker:        bannedModalChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:   enforcement,
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