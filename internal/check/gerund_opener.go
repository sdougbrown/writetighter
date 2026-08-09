package check

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

type gerundOpenerChecker struct{}

func (gerundOpenerChecker) ID() string   { return "CORE.GERUND_OPENER" }
func (gerundOpenerChecker) Version() int { return 1 }

func (gerundOpenerChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}

	blocks := document.AnalyzeProse(ctx.Document)
	var out []report.Finding

	for _, block := range blocks {
		// Skip list items, headings, and blockquotes — gerund openers are
		// legitimate in instructions ("Configuring the server...") and
		// headings. Only flag standalone prose paragraphs.
		if block.Marker != "" {
			continue
		}
		sentences := document.SentenceUnits(block, ctx.Document.Content)
		for _, s := range sentences {
			tokens := document.ExportLexicalTokens(s.Text)
			if len(tokens) < 2 {
				continue
			}
			first := strings.ToLower(tokens[0])
			second := strings.ToLower(tokens[1])
			if !strings.HasSuffix(first, "ing") {
				continue
			}
			// Exclude words ending in "-ing" that are not gerunds: short
			// words like "ring", "sing" and base verbs with short stems
			// like "bring", "fling", "swing". Require at least 3 characters
			// before the "-ing" suffix, so the word must be 6+ runes.
			if utf8.RuneCountInString(first) < 6 {
				continue
			}
			if !isGerundDeterminer(second, ctx) {
				continue
			}
			path := ctx.Document.Source
			rng := &report.FindingRange{
				StartByte: s.StartByte, EndByte: s.EndByte,
				StartLine: s.StartLine, StartColumn: s.StartColumn,
				EndLine: s.EndLine, EndColumn: s.EndColumn,
			}
			out = append(out, report.Finding{
				RuleID:         gerundOpenerChecker{}.ID(),
				RuleVersion:    1,
				Checker:        gerundOpenerChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    "candidate",
				Severity:       "info",
				Path:           &path,
				Range:          rng,
				Evidence:       fmt.Sprintf("sentence opens with gerund %q", tokens[0]),
				Message:        "Sentence opens with a gerund. Use an imperative for instructions or a declarative subject for descriptions.",
				Confidence:     1,
			})
		}
	}
	return out, nil
}

// isGerundDeterminer checks whether word is a determiner that signals a gerund
// phrase opener. It reads from the profile dictionary's "gerund_determiner"
// word class, falling back to a minimal hardcoded set for v1 profiles.
func isGerundDeterminer(word string, ctx *RunContext) bool {
	if ctx.Profile != nil && ctx.Profile.Dict != nil && ctx.Profile.Dict.HasWordClass(word, "gerund_determiner") {
		return true
	}
	// Fallback for v1 profiles without word classes.
	switch word {
	case "the", "a", "an", "this", "that", "your", "our", "their",
		"his", "her", "its", "some", "any", "every", "each",
		"all", "no", "these", "those":
		return true
	}
	return false
}

func init() { Register(gerundOpenerChecker{}) }
