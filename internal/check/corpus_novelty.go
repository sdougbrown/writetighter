package check

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/corpus"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

// CORE.CORPUS_NOVELTY is an opt-in, advisory-only checker that flags content
// words and bounded phrases appearing in the changed prose but absent from a
// declared Git comparison revision. It asks for clarification (definition,
// established name, or plainer wording). It does NOT declare a term invalid,
// does NOT modify terminology status, and does NOT act as a vocabulary gate.
//
// The checker abstains entirely when RunContext.GitCompare is nil (no --git-compare
// flag was passed). It is disabled by default in the profile rules.

// corpusNoveltyStopwords are now defined in the profile dictionary under the
// "corpus_novelty_stopword" word class, loaded in isExcludedToken below.

type corpusNoveltyChecker struct{}

func (corpusNoveltyChecker) ID() string   { return "CORE.CORPUS_NOVELTY" }
func (corpusNoveltyChecker) Version() int { return 1 }

func (corpusNoveltyChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.GitCompare == nil {
		return nil, nil
	}

	minRepetition := 2
	for _, r := range ctx.Profile.Rules.Rules {
		if r.ID != "CORE.CORPUS_NOVELTY" {
			continue
		}
		if r.Parameters == nil {
			continue
		}
		if v, ok := r.Parameters["min_repetition"]; ok {
			if n, ok2 := toInt(v); ok2 && n > 0 {
				minRepetition = n
			}
		}
	}

	// Load stopword class from the profile dictionary.
	var stopwords map[string]bool
	if ctx.Profile != nil && ctx.Profile.Dict != nil {
		stopwords = ctx.Profile.Dict.WordClassSet("corpus_novelty_stopword")
	}

	enforcement, severity := ruleEnforcement(ctx, corpusNoveltyChecker{}.ID())

	var out []report.Finding
	reported := make(map[string]bool) // dedupe by normalized term within this document

	revStr := safeRevisionPrefix(ctx.GitCompare.Revision)

	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		text := seg.Text

		// --- Token pass ---
		rawTokens := extractRawTokens(text)
		for _, rt := range rawTokens {
			lt := corpus.FoldUnicode(rt)
			if reported[lt] {
				continue
			}
			if isExcludedToken(rt, lt, ctx, stopwords) {
				continue
			}
			changeCount := ctx.GitCompare.ChangeTermCounts[lt]
			if changeCount < minRepetition {
				continue
			}
			gitCompareCount := ctx.GitCompare.TermCounts[lt]
			if gitCompareCount > 0 {
				continue
			}
			reported[lt] = true
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         corpusNoveltyChecker{}.ID(),
				RuleVersion:    1,
				Checker:        corpusNoveltyChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range: &report.FindingRange{
					StartByte:   seg.Range.Start.Byte,
					EndByte:     seg.Range.End.Byte,
					StartLine:   seg.Range.Start.Line,
					StartColumn: seg.Range.Start.Column,
					EndLine:     seg.Range.End.Line,
					EndColumn:   seg.Range.End.Column,
				},
				Evidence: fmt.Sprintf("corpus-novelty: term %q git_compare_count=0 change_count=%d git_compare_rev=%s",
					lt, changeCount, ctx.GitCompare.Revision),
				Message: fmt.Sprintf("Term %q appears %d time(s) in the changed prose but has no precedent in the comparison revision (%s). "+
					"If this is an established project term, consider documenting it. If not, consider plainer wording.",
					lt, changeCount, revStr),
				Confidence: 1,
			})
		}

		// --- Phrase pass (2–3 word phrases) ---
		phrases := corpus.ExtractPhrases(text)
		for _, phrase := range phrases {
			if reported[phrase] {
				continue
			}
			if isExcludedPhrase(phrase, ctx, stopwords) {
				continue
			}
			changeCount := ctx.GitCompare.ChangePhraseCounts[phrase]
			if changeCount < minRepetition {
				continue
			}
			gitCompareCount := ctx.GitCompare.PhraseCounts[phrase]
			if gitCompareCount > 0 {
				continue
			}
			reported[phrase] = true
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         corpusNoveltyChecker{}.ID(),
				RuleVersion:    1,
				Checker:        corpusNoveltyChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range: &report.FindingRange{
					StartByte:   seg.Range.Start.Byte,
					EndByte:     seg.Range.End.Byte,
					StartLine:   seg.Range.Start.Line,
					StartColumn: seg.Range.Start.Column,
					EndLine:     seg.Range.End.Line,
					EndColumn:   seg.Range.End.Column,
				},
				Evidence: fmt.Sprintf("corpus-novelty: phrase %q git_compare_count=0 change_count=%d git_compare_rev=%s",
					phrase, changeCount, ctx.GitCompare.Revision),
				Message: fmt.Sprintf("Phrase %q appears %d time(s) in the changed prose but has no precedent in the comparison revision (%s). "+
					"If this is an established project term, consider documenting it. If not, consider plainer wording.",
					phrase, changeCount, revStr),
				Confidence: 1,
			})
		}
	}
	return out, nil
}

// extractRawTokens splits text into word tokens preserving original case.
// Hyphens within words are preserved.
func extractRawTokens(text string) []string {
	runes := []rune(text)
	var tokens []string
	i := 0
	for i < len(runes) {
		r := runes[i]
		if !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			i++
			continue
		}
		start := i
		for i < len(runes) {
			r := runes[i]
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '\'' || r == '\u2019' {
				i++
				continue
			}
			break
		}
		raw := string(runes[start:i])
		raw = strings.TrimFunc(raw, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if raw == "" {
			continue
		}
		tokens = append(tokens, raw)
	}
	return tokens
}

// safeRevisionPrefix returns up to the first 8 characters of revision,
// falling back to the full revision when it is shorter than 8 chars.
// This prevents panics on short revisions used in message construction.
func safeRevisionPrefix(revision string) string {
	if len(revision) <= 8 {
		return revision
	}
	return revision[:8]
}

// isExcludedToken checks all exclusion rules for a single token.
func isExcludedToken(raw, lower string, ctx *RunContext, stopwords map[string]bool) bool {
	// 1. Identifier detection (before lowercasing — on raw token)
	if corpus.IsIdentifier(raw) {
		return true
	}
	// 2. URL/path/issue/version detection
	if corpus.IsURLOrPath(raw) {
		return true
	}
	// 3. Project term and profile dictionary precedence
	if ctx.Profile != nil && ctx.Profile.Dict != nil {
		if e := profile.ResolveTerm(ctx.Profile.Dict, ctx.Terms, lower); e != nil {
			return true
		}
	}
	// 4. Common English stopwords (from profile dictionary)
	if stopwords[lower] {
		return true
	}
	// 5. Too short
	if utf8.RuneCountInString(lower) < 3 {
		return true
	}
	return false
}

// isExcludedPhrase checks exclusion for a phrase by examining each word.
func isExcludedPhrase(phrase string, ctx *RunContext, stopwords map[string]bool) bool {
	words := strings.Fields(phrase)
	for _, w := range words {
		if isExcludedToken(w, w, ctx, stopwords) {
			return true
		}
	}
	return false
}

func init() { Register(corpusNoveltyChecker{}) }
