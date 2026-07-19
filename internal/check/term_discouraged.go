package check

import (
	"fmt"
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
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, entry := range ctx.Profile.Dict.Entries {
			if entry.Status != profile.StatusDiscouraged {
				continue
			}
			needle := strings.ToLower(entry.Term)
			text := strings.ToLower(seg.Text)
			idx := strings.Index(text, needle)
			if idx < 0 {
				continue
			}
			end := idx + len(entry.Term)
			path := ctx.Document.Source
			suggestion := strings.Join(entry.Alternatives, ", ")
			out = append(out, report.Finding{
				RuleID:         termDiscouragedChecker{}.ID(),
				RuleVersion:    1,
				Checker:        termDiscouragedChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    "enforced",
				Severity:       "warning",
				Path:           &path,
				Range:          &report.FindingRange{StartByte: seg.Range.Start.Byte + idx, EndByte: seg.Range.Start.Byte + end, StartLine: seg.Range.Start.Line, StartColumn: seg.Range.Start.Column + idx, EndLine: seg.Range.Start.Line, EndColumn: seg.Range.Start.Column + end},
				Evidence:       fmt.Sprintf("'%s' is discouraged; use '%s' instead", entry.Term, suggestion),
				Message:        entry.Reason,
				Suggestion:     &suggestion,
				Confidence:     1,
			})
		}
	}
	return out, nil
}
func init() { Register(termDiscouragedChecker{}) }
