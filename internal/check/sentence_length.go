package check

import (
	"fmt"
	"strconv"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/report"
)

type sentenceLengthChecker struct{}

func (sentenceLengthChecker) ID() string   { return "CORE.SENTENCE_LENGTH" }
func (sentenceLengthChecker) Version() int { return 1 }
func (sentenceLengthChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	return sentenceLengthRun(ctx)
}

func sentenceLengthRun(ctx *RunContext) ([]report.Finding, error) {
	limit := 0
	for _, r := range ctx.Profile.Rules.Rules {
		if r.ID != "CORE.SENTENCE_LENGTH" {
			continue
		}
		if r.Parameters == nil {
			continue
		}
		if v, ok := r.Parameters[guidance.SentenceLimitParameter(ctx.Document.Kind)]; ok {
			limit, _ = toInt(v)
		}
	}
	if limit == 0 {
		return nil, nil
	}

	// Use the shared prose analysis layer to derive sentence units.
	blocks := document.AnalyzeProse(ctx.Document)
	var out []report.Finding

	for _, block := range blocks {
		sentences := document.SentenceUnits(block, ctx.Document.Content)
		for _, s := range sentences {
			if s.WordCount > limit {
				msg := "Split this sentence into independently verifiable claims."
				evidence := fmt.Sprintf("%d words; profile limit is %d", s.WordCount, limit)
				path := ctx.Document.Source
				rng := &report.FindingRange{
					StartByte: s.StartByte, EndByte: s.EndByte,
					StartLine: s.StartLine, StartColumn: s.StartColumn,
					EndLine: s.EndLine, EndColumn: s.EndColumn,
				}
				out = append(out, report.Finding{
					RuleID: sentenceLengthChecker{}.ID(), RuleVersion: 1,
					Checker: sentenceLengthChecker{}.ID(), CheckerVersion: 1,
					Enforcement: "enforced", Severity: "warning",
					Path: &path, Range: rng,
					Evidence: evidence, Message: msg, Confidence: 1,
				})
			}
		}
	}
	return out, nil
}

func toInt(v any) (int, bool) {
	i, err := strconv.Atoi(fmt.Sprint(v))
	return i, err == nil
}

func init() { Register(sentenceLengthChecker{}) }
