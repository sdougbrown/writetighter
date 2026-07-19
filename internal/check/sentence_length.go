package check

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
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
		if v, ok := r.Parameters[ctx.Document.Kind+"_max_words"]; ok {
			limit, _ = toInt(v)
		}
	}
	if limit == 0 {
		return nil, nil
	}
	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		wc := wordCount(seg.Text)
		if wc > limit {
			msg := "Split this sentence into independently verifiable claims."
			evidence := fmt.Sprintf("%d words; profile limit is %d", wc, limit)
			path := ctx.Document.Source
			rng := &report.FindingRange{StartByte: seg.Range.Start.Byte, EndByte: seg.Range.End.Byte, StartLine: seg.Range.Start.Line, StartColumn: seg.Range.Start.Column, EndLine: seg.Range.End.Line, EndColumn: seg.Range.End.Column}
			out = append(out, report.Finding{RuleID: sentenceLengthChecker{}.ID(), RuleVersion: 1, Checker: sentenceLengthChecker{}.ID(), CheckerVersion: 1, Enforcement: "enforced", Severity: "warning", Path: &path, Range: rng, Evidence: evidence, Message: msg, Confidence: 1})
		}
	}
	return out, nil
}

func toInt(v any) (int, bool) { i, err := strconv.Atoi(fmt.Sprint(v)); return i, err == nil }

func wordCount(s string) int { return len(strings.Fields(s)) }

func init() { Register(sentenceLengthChecker{}) }
