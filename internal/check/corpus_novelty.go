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
// declared baseline revision. It asks for clarification (definition,
// established name, or plainer wording). It does NOT declare a term invalid,
// does NOT modify terminology status, and does NOT act as a vocabulary gate.
//
// The checker abstains entirely when RunContext.Baseline is nil (no --git-compare
// flag was passed). It is disabled by default in the profile rules.

// corpusNoveltyStopwords are function words and common English words that
// should never be flagged as coined terms. This reuses the same closed set
// as proseStopwords from noun_stack.go, extended with a few additional common
// words observed in the investigation fixtures.
var corpusNoveltyStopwords = map[string]bool{
	// articles
	"a": true, "an": true, "the": true,
	// prepositions
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "into": true, "onto": true,
	"upon": true, "over": true, "under": true, "through": true,
	"between": true, "among": true, "across": true, "against": true,
	"about": true, "as": true, "than": true, "per": true, "past": true,
	"without": true, "within": true, "during": true, "including": true,
	"via": true, "using": true, "despite": true, "toward": true,
	"towards": true, "behind": true, "beyond": true,
	// conjunctions
	"and": true, "or": true, "but": true, "nor": true, "yet": true,
	"so": true, "if": true, "then": true, "when": true, "while": true,
	"where": true, "because": true, "although": true, "though": true,
	"since": true, "unless": true, "until": true, "before": true, "after": true,
	"whether": true, "once": true, "whereas": true,
	// pronouns and determiners
	"it": true, "its": true, "they": true, "them": true, "their": true,
	"there": true, "here": true, "this": true, "that": true,
	"these": true, "those": true, "which": true, "who": true, "whom": true,
	"whose": true, "what": true, "his": true, "her": true,
	"our": true, "your": true, "my": true, "we": true, "us": true,
	"you": true, "i": true, "he": true, "him": true, "she": true, "me": true,
	"some": true, "any": true, "all": true, "each": true, "both": true,
	"either": true, "neither": true, "every": true, "few": true, "many": true,
	"much": true, "more": true, "most": true, "less": true, "least": true,
	"other": true, "another": true, "same": true, "such": true, "own": true,
	// auxiliary and modal verbs
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "am": true, "do": true, "does": true,
	"did": true, "have": true, "has": true, "had": true, "having": true,
	"will": true, "would": true, "can": true, "could": true,
	"should": true, "shall": true, "may": true, "might": true, "must": true,
	// common adverbs and hedges
	"not": true, "no": true, "also": true, "just": true,
	"only": true, "even": true, "still": true, "already": true,
	"never": true, "always": true, "often": true, "sometimes": true,
	"now": true, "too": true, "again": true,
	"very": true, "really": true, "quite": true, "rather": true,
	"however": true, "therefore": true, "thus": true, "hence": true,
	"indeed": true, "actually": true, "simply": true,
	"particularly": true, "especially": true, "specifically": true,
	"generally": true, "typically": true, "usually": true,
	"commonly": true, "normally": true, "previously": true,
	"currently": true, "subsequently": true, "finally": true,
	"next": true, "instead": true, "perhaps": true, "maybe": true,
	"likely": true, "basically": true, "essentially": true,
	// common contractions
	"isn't": true, "aren't": true, "wasn't": true, "weren't": true,
	"don't": true, "doesn't": true, "didn't": true, "won't": true,
	"wouldn't": true, "can't": true, "couldn't": true, "shouldn't": true,
	"hasn't": true, "haven't": true, "hadn't": true,
	"it's": true, "they're": true, "we're": true, "you're": true,
	"that's": true, "there's": true, "here's": true, "what's": true,
	"who's": true, "let's": true,
	// common verbs (from noun_stack.go nounRunFinalVerbs — these are ordinary)
	"use": true, "uses": true, "used": true,
	"keep": true, "keeps": true, "kept": true,
	"stay": true, "stays": true, "stayed": true,
	"read": true, "reads": true,
	"match": true, "matches": true, "matched": true,
	"send": true, "sends": true, "sent": true,
	"pass": true, "passes": true, "passed": true,
	"set": true, "sets": true,
	"skip": true, "skips": true, "skipped": true,
	"run": true, "runs": true, "ran": true,
	"make": true, "makes": true, "made": true,
	"take": true, "takes": true, "took": true,
	"get": true, "gets": true, "got": true,
	"see": true, "sees": true, "saw": true,
	"come": true, "comes": true, "came": true,
	"go": true, "goes": true, "went": true,
	"hold": true, "holds": true, "held": true,
	"show": true, "shows": true, "showed": true,
	"mean": true, "means": true, "meant": true,
	"tell": true, "tells": true, "told": true,
	"put": true, "puts": true,
	"leave": true, "leaves": true, "left": true,
	"bring": true, "brings": true, "brought": true,
	"work": true, "works": true, "worked": true,
	"turn": true, "turns": true, "turned": true,
	"begin": true, "begins": true, "began": true, "begun": true,
	"call": true, "calls": true, "called": true,
	"return": true, "returns": true, "returned": true,
	"allow": true, "allows": true, "allowed": true,
	"follow": true, "follows": true, "followed": true,
	"cause": true, "causes": true, "caused": true,
	"change": true, "changes": true, "changed": true,
	"fail": true, "fails": true, "failed": true,
	"exist": true, "exists": true, "existed": true,
	"appear": true, "appears": true, "appeared": true,
	"remain": true, "remains": true, "remained": true,
	"happen": true, "happens": true, "happened": true,
	"occur": true, "occurs": true, "occurred": true,
	"become": true, "becomes": true, "became": true,
	"seem": true, "seems": true, "seemed": true,
	"slip": true, "slips": true, "slipped": true,
	"throw": true, "throws": true, "threw": true,
	"choose": true, "chooses": true, "chose": true, "chosen": true,
	"parse": true, "parses": true, "parsed": true,
	"strip": true, "strips": true, "stripped": true,
	"register": true, "registers": true, "registered": true,
	"resolve": true, "resolves": true, "resolved": true,
	"update": true, "updates": true, "updated": true,
	"check": true, "checks": true, "checked": true,
	"drop": true, "drops": true, "dropped": true,
	"override": true, "overrides": true, "overridden": true,
	"trigger": true, "triggers": true, "triggered": true,
	// additional common words from investigation
	"new": true, "null": true, "out": true,
	"building": true, "partial": true, "test": true,
	"provides": true, "factory": true, "keys": true,
}

type corpusNoveltyChecker struct{}

func (corpusNoveltyChecker) ID() string   { return "CORE.CORPUS_NOVELTY" }
func (corpusNoveltyChecker) Version() int { return 1 }

func (corpusNoveltyChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.Baseline == nil {
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

	enforcement, severity := ruleEnforcement(ctx, corpusNoveltyChecker{}.ID())

	var out []report.Finding
	reported := make(map[string]bool) // dedupe by normalized term within this document

	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		text := seg.Text

		// --- Token pass ---
		rawTokens := extractRawTokens(text)
		for _, rt := range rawTokens {
			lt := foldToken(rt)
			if reported[lt] {
				continue
			}
			if isExcludedToken(rt, lt, ctx) {
				continue
			}
			changeCount := ctx.Baseline.ChangeTermCounts[lt]
			if changeCount < minRepetition {
				continue
			}
			baselineCount := ctx.Baseline.TermCounts[lt]
			if baselineCount > 0 {
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
				Evidence: fmt.Sprintf("corpus-novelty: term %q baseline_count=0 change_count=%d baseline_rev=%s",
					lt, changeCount, ctx.Baseline.Revision),
				Message: fmt.Sprintf("Term %q appears %d time(s) in the changed prose but has no precedent in the baseline (revision %s). "+
					"If this is an established project term, consider documenting it. If not, consider plainer wording.",
					lt, changeCount, ctx.Baseline.Revision[:8]),
				Confidence: 1,
			})
		}

		// --- Phrase pass (2–3 word phrases) ---
		phrases := extractPhrases(text)
		for _, phrase := range phrases {
			if reported[phrase] {
				continue
			}
			if isExcludedPhrase(phrase, ctx) {
				continue
			}
			changeCount := ctx.Baseline.ChangePhraseCounts[phrase]
			if changeCount < minRepetition {
				continue
			}
			baselineCount := ctx.Baseline.PhraseCounts[phrase]
			if baselineCount > 0 {
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
				Evidence: fmt.Sprintf("corpus-novelty: phrase %q baseline_count=0 change_count=%d baseline_rev=%s",
					phrase, changeCount, ctx.Baseline.Revision),
				Message: fmt.Sprintf("Phrase %q appears %d time(s) in the changed prose but has no precedent in the baseline (revision %s). "+
					"If this is an established project term, consider documenting it. If not, consider plainer wording.",
					phrase, changeCount, ctx.Baseline.Revision[:8]),
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

// foldToken lowercases a token via Unicode case folding.
func foldToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// extractPhrases extracts bounded 2–3 word phrases from text, lowercased.
func extractPhrases(text string) []string {
	var b strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == ' ' || r == '\n' || r == '\t' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	words := strings.Fields(b.String())
	var phrases []string
	for i := range words {
		for n := 2; n <= 3 && i+n <= len(words); n++ {
			phrase := foldToken(strings.Join(words[i:i+n], " "))
			phrases = append(phrases, phrase)
		}
	}
	return phrases
}

// isExcludedToken checks all exclusion rules for a single token.
func isExcludedToken(raw, lower string, ctx *RunContext) bool {
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
	// 4. Common English stopwords
	if corpusNoveltyStopwords[lower] {
		return true
	}
	// 5. Too short
	if utf8.RuneCountInString(lower) < 3 {
		return true
	}
	return false
}

// isExcludedPhrase checks exclusion for a phrase by examining each word.
func isExcludedPhrase(phrase string, ctx *RunContext) bool {
	words := strings.Fields(phrase)
	for _, w := range words {
		if isExcludedToken(w, w, ctx) {
			return true
		}
	}
	return false
}

func init() { Register(corpusNoveltyChecker{}) }