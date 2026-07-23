package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

type termDiscouragedChecker struct{}

func (termDiscouragedChecker) ID() string   { return "CORE.TERM_DISCOURAGED" }
func (termDiscouragedChecker) Version() int { return 1 }

func (termDiscouragedChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.Profile == nil || ctx.Profile.Dict == nil {
		return nil, nil
	}
	var out []report.Finding
	enforcement, severity := "enforced", "warning"
	if ctx.Profile.Rules != nil {
		for _, rule := range ctx.Profile.Rules.Rules {
			if rule.ID != (termDiscouragedChecker{}).ID() {
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

	// Collect and sort discouraged entries by term byte length (longest first).
	var terms []profile.Entry
	for _, entry := range ctx.Profile.Dict.Entries {
		if entry.Status == profile.StatusDiscouraged {
			terms = append(terms, entry)
		}
	}
	sort.Slice(terms, func(i, j int) bool {
		return len(terms[i].Term) > len(terms[j].Term)
	})

	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		// Track covered byte positions to avoid overlapping matches.
		covered := make([]bool, len(seg.Text))

		for _, entry := range terms {
			for _, match := range insensitiveMatches(seg.Text, entry.Term) {
				actualPos, matchEnd := match[0], match[1]

				// Check if any position in this match range is already covered.
				skip := false
				for i := actualPos; i < matchEnd; i++ {
					if covered[i] {
						skip = true
						break
					}
				}
				if skip {
					continue
				}

				// Mark all positions as covered.
				for i := actualPos; i < matchEnd; i++ {
					covered[i] = true
				}

				path := ctx.Document.Source
				evidence := fmt.Sprintf("'%s' is discouraged", entry.Term)
				var suggestion *string
				if len(entry.Alternatives) > 0 {
					joined := strings.Join(entry.Alternatives, ", ")
					evidence += fmt.Sprintf("; use '%s' instead", joined)
					suggestion = &joined
				}
				out = append(out, report.Finding{
					RuleID:         termDiscouragedChecker{}.ID(),
					RuleVersion:    1,
					Checker:        termDiscouragedChecker{}.ID(),
					CheckerVersion: 1,
					Enforcement:    enforcement,
					Severity:       severity,
					Path:           &path,
					Range: &report.FindingRange{
						StartByte: seg.Range.Start.Byte + actualPos,
						EndByte:   seg.Range.Start.Byte + matchEnd,
						StartLine: seg.Range.Start.Line, StartColumn: codePointColumn(seg.Text, actualPos, seg.Range.Start.Column),
						EndLine: seg.Range.Start.Line, EndColumn: codePointColumn(seg.Text, matchEnd, seg.Range.Start.Column),
					},
					Evidence:   evidence,
					Message:    entry.Reason,
					Suggestion: suggestion,
					Confidence: 1,
				})
			}
		}
	}
	return out, nil
}
func init() { Register(termDiscouragedChecker{}) }
