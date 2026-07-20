package check

import (
	"fmt"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

type denseParagraphChecker struct{}

func (denseParagraphChecker) ID() string   { return "CORE.DENSE_PARAGRAPH" }
func (denseParagraphChecker) Version() int { return 1 }
func (denseParagraphChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.Profile == nil || ctx.Profile.Rules == nil {
		return nil, nil
	}

	// Load profile-configured thresholds.
	// Default to disabled-like behavior (high thresholds) if not configured.
	maxSentences := 3 // default
	maxWords := 50    // default

	for _, r := range ctx.Profile.Rules.Rules {
		if r.ID != "CORE.DENSE_PARAGRAPH" {
			continue
		}
		if r.Parameters == nil {
			continue
		}
		if v, ok := r.Parameters["max_sentences"]; ok {
			if n, ok2 := toInt(v); ok2 && n > 0 {
				maxSentences = n
			}
		}
		if v, ok := r.Parameters["max_words"]; ok {
			if n, ok2 := toInt(v); ok2 && n > 0 {
				maxWords = n
			}
		}
	}

	// Use the shared prose analysis layer.
	blocks := document.AnalyzeProse(ctx.Document)
	var out []report.Finding

	for _, block := range blocks {
		sentences := document.SentenceUnits(block, ctx.Document.Content)
		nSentences := len(sentences)
		// Total words across all sentences in this paragraph.
		totalWords := 0
		for _, s := range sentences {
			totalWords += s.WordCount
		}

		if nSentences > maxSentences || totalWords > maxWords {
			path := ctx.Document.Source
			evidence := fmt.Sprintf("%d sentences; %d words", nSentences, totalWords)
			rng := &report.FindingRange{
				StartByte: block.StartByte, EndByte: block.EndByte,
				StartLine: block.StartLine, StartColumn: block.StartColumn,
				EndLine: block.EndLine, EndColumn: block.EndColumn,
			}
			out = append(out, report.Finding{
				RuleID: denseParagraphChecker{}.ID(), RuleVersion: 1,
				Checker: denseParagraphChecker{}.ID(), CheckerVersion: 1,
				Enforcement: "candidate", Severity: "info",
				Path: &path, Range: rng,
				Evidence: evidence, Message: "Paragraph is dense.", Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(denseParagraphChecker{}) }
