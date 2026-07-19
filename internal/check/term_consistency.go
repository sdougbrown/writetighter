package check

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

type termConsistencyChecker struct{}

func (termConsistencyChecker) ID() string   { return "CORE.TERM_CONSISTENCY" }
func (termConsistencyChecker) Version() int { return 1 }
func (termConsistencyChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	seen := map[string]string{}
	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, raw := range strings.Fields(seg.Text) {
			term := strings.Trim(raw, "()[]{}.,;:!?'\"")
			if term == "" {
				continue
			}
			key := strings.ToLower(strings.ReplaceAll(term, "-", ""))
			if prev, ok := seen[key]; ok && prev != term {
				path := ctx.Document.Source
				out = append(out, report.Finding{RuleID: termConsistencyChecker{}.ID(), RuleVersion: 1, Checker: termConsistencyChecker{}.ID(), CheckerVersion: 1, Enforcement: "candidate", Severity: "info", Path: &path, Evidence: fmt.Sprintf("term spelled as %q and %q", prev, term), Message: "Term spelling is inconsistent.", Confidence: 1})
				continue
			}
			seen[key] = term
		}
	}
	return out, nil
}
func init() { Register(termConsistencyChecker{}) }
