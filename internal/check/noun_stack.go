package check

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

// stackRun represents a maximal run of consecutive content (non-stopword)
// tokens and the token that immediately preceded the run, if any.
type stackRun struct {
	tokens         []string
	precedingToken string // lowered; empty if run is at sentence start or after a clause boundary
}

// boundary runes that end a clause. A content-word run may never cross one, so
// "Case size changed — rebuild the measures" can never collapse into a single
// stack even though the em dash is not a sentence terminator.
func isClauseBoundary(r rune) bool {
	switch r {
	case '—', '–', ';', ':':
		return true
	}
	return false
}

// word class names used by nounStackChecker. These are keys into the profile
// dictionary's word_classes map.
const (
	wordClassStopword                = "stopword"
	wordClassFiniteVerb              = "finite_verb"
	wordClassIrregularParticiple     = "irregular_participle"
	wordClassParticipleHeadException = "participle_head_exception"
	wordClassGardenPathHead          = "garden_path_head"
	wordClassDeterminer              = "determiner"
)

// scanNounStackRuns walks a sentence and returns maximal runs of consecutive
// content words. A word is content if it is NOT in the given stopword set.
// Runs are split at clause boundaries, so a window never spans an em/en dash,
// semicolon, or colon. Each run carries the stopword that preceded it, if any.
func scanNounStackRuns(text string, stopwords map[string]bool) []stackRun {
	runes := []rune(text)
	n := len(runes)
	var runs []stackRun
	var cur []string
	var lastStopword string // most recent stopword seen (lowered)
	i := 0
	for i < n {
		r := runes[i]
		switch {
		case isClauseBoundary(r):
			if len(cur) > 0 {
				runs = append(runs, stackRun{tokens: cur, precedingToken: lastStopword})
				cur = nil
			}
			lastStopword = ""
			i++
		case !(unicode.IsLetter(r) || unicode.IsDigit(r)):
			i++
		default:
			start := i
			i++
			for i < n {
				rr := runes[i]
				if unicode.IsLetter(rr) || unicode.IsDigit(rr) {
					i++
					continue
				}
				if rr == '\'' || rr == '’' || rr == '-' {
					if i+1 < n && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1])) {
						i++
						continue
					}
				}
				break
			}
			tok := string(runes[start:i])
			lowerTok := strings.ToLower(tok)
			if stopwords[lowerTok] {
				if len(cur) > 0 {
					runs = append(runs, stackRun{tokens: cur, precedingToken: lastStopword})
					cur = nil
				}
				lastStopword = lowerTok
				continue
			}
			cur = append(cur, tok)
		}
	}
	if len(cur) > 0 {
		runs = append(runs, stackRun{tokens: cur, precedingToken: lastStopword})
	}
	return runs
}

type nounStackChecker struct{}

func (nounStackChecker) ID() string   { return "CORE.NOUN_STACK" }
func (nounStackChecker) Version() int { return 1 }

func (nounStackChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}

	threshold := 3
	if ctx.Profile != nil && ctx.Profile.Rules != nil {
		for _, r := range ctx.Profile.Rules.Rules {
			if r.ID != "CORE.NOUN_STACK" {
				continue
			}
			if r.Parameters == nil {
				continue
			}
			if v, ok := r.Parameters["min_stack_length"]; ok {
				if n, ok2 := toInt(v); ok2 && n > 0 {
					threshold = n
				}
			}
		}
	}

	// Load word classes from the profile dictionary.
	// When the dictionary does not carry a class the map will be nil, which
	// makes lookups return false — graceful degradation.
	var stopwords, finiteVerbs, irregularParts, partExc, gpHd map[string]bool
	if ctx.Profile != nil && ctx.Profile.Dict != nil {
		stopwords = ctx.Profile.Dict.WordClassSet(wordClassStopword)
		finiteVerbs = ctx.Profile.Dict.WordClassSet(wordClassFiniteVerb)
		irregularParts = ctx.Profile.Dict.WordClassSet(wordClassIrregularParticiple)
		partExc = ctx.Profile.Dict.WordClassSet(wordClassParticipleHeadException)
		gpHd = ctx.Profile.Dict.WordClassSet(wordClassGardenPathHead)
	}

	// Identifier/proper-name-heavy stacks are only suppressed in code-adjacent
	// prose; plain-language docs keep the full check.
	codeAdjacent := ctx.Document.Kind == guidance.KindCodeComment || ctx.Document.Kind == guidance.KindPR

	blocks := document.AnalyzeProse(ctx.Document)
	var out []report.Finding

	for _, block := range blocks {
		// Skip headings, list items, and blockquotes — noun stacks in
		// these contexts are intentional shorthand, not prose problems.
		if block.Marker != "" {
			continue
		}
		sentences := document.SentenceUnits(block, ctx.Document.Content)
		for _, s := range sentences {
			runs := scanNounStackRuns(s.Text, stopwords)
			for _, run := range runs {
				if len(run.tokens) < threshold {
					continue
				}
				if !isNounPhraseRun(run.tokens, finiteVerbs, irregularParts, partExc) {
					continue
				}
				if codeAdjacent && stackHasIdentifier(run.tokens) {
					continue
				}
				stack := strings.Join(run.tokens, " ")
				path := ctx.Document.Source
				rng := &report.FindingRange{
					StartByte: s.StartByte, EndByte: s.EndByte,
					StartLine: s.StartLine, StartColumn: s.StartColumn,
					EndLine: s.EndLine, EndColumn: s.EndColumn,
				}

				// Determine if this stack has a garden-path structure:
				// a determiner followed by a verb/noun homograph at the head
				// of the content-word run, which makes the phrase boundaries
				// structurally ambiguous.
				var isAmbiguous bool
				if ctx.Profile != nil && ctx.Profile.Dict != nil {
					isAmbiguous = stackHasGardenPathHead(run, gpHd, ctx.Profile.Dict)
				}

				evidence := fmt.Sprintf("noun stack (%d content words): %q", len(run.tokens), stack)
				msg := "Long noun stack. Consider unpacking into subject-verb-object."
				if isAmbiguous {
					headWord := run.tokens[0]
					evidence = fmt.Sprintf("ambiguous noun stack (%d content words): %q (head %q is a verb/noun homograph after a determiner)", len(run.tokens), stack, headWord)
					msg = fmt.Sprintf("This noun sequence follows a determiner and starts with %q, which could be a verb or a noun, making the phrase boundaries structurally ambiguous. Consider restructuring to clarify the grammatical relationships, e.g. 'Pay attention to the pruning logic.'", headWord)
				}

				out = append(out, report.Finding{
					RuleID:         nounStackChecker{}.ID(),
					RuleVersion:    1,
					Checker:        nounStackChecker{}.ID(),
					CheckerVersion: 1,
					Enforcement:    "candidate",
					Severity:       "info",
					Path:           &path,
					Range:          rng,
					Evidence:       evidence,
					Message:        msg,
					Confidence:     1,
				})
			}
		}
	}
	return out, nil
}

// isNounPhraseRun rejects runs that read as clauses rather than noun phrases:
// any frequent finite verb anywhere in the window ("overrides stay partial",
// "then passes") or a participle closing the window ("barcode registered").
// Ending -ed/-en forms only count at the final position so legitimate stacks
// like "Localized display name" or "left-aligned text" survive.
func isNounPhraseRun(run []string, finiteVerbs, irregularParts, partExc map[string]bool) bool {
	for _, tok := range run {
		if finiteVerbs[lower(tok)] {
			return false
		}
	}
	return !endsInParticiple(run[len(run)-1], irregularParts, partExc)
}

func identifierOrProperNounToken(token string) bool {
	hasDigit := false
	uppercaseCount := 0
	for i, r := range token {
		if unicode.IsDigit(r) {
			hasDigit = true
		}
		if unicode.IsUpper(r) {
			uppercaseCount++
			if i > 0 {
				return true
			}
		}
	}
	if hasDigit {
		return true
	}
	n := utf8.RuneCountInString(token)
	return uppercaseCount >= 2 && uppercaseCount <= 5 && uppercaseCount == n
}

// endsInParticiple checks whether token is a past participle that would close
// a reduced clause ("barcode registered", "class chosen"). Words in partExc
// (participle head exceptions) are English nouns that happen to end in -ed
// but are not participles.
func endsInParticiple(token string, irregularParts, partExc map[string]bool) bool {
	lower := strings.ToLower(token)
	if irregularParts[lower] {
		return true
	}
	return len(lower) >= 4 && strings.HasSuffix(lower, "ed") && !partExc[lower]
}

func stackHasIdentifier(run []string) bool {
	for _, tok := range run {
		if identifierOrProperNounToken(tok) {
			return true
		}
	}
	return false
}

// stackHasGardenPathHead reports whether a stack run has the garden-path
// structure: preceded by a determiner, with a first token that is a common
// verb/noun homograph. The reader cannot immediately tell whether the first
// word is a verb continuing the clause or a noun starting a new noun phrase.
func stackHasGardenPathHead(run stackRun, gpHd map[string]bool, dict *profile.Dictionary) bool {
	if len(run.tokens) == 0 {
		return false
	}
	// Check that the preceding token is a determiner.
	if !isDeterminerFromDict(run.precedingToken, dict) {
		return false
	}
	return gpHd[lower(run.tokens[0])]
}

// isDeterminerFromDict checks whether s is classified as a determiner in the
// profile dictionary. The embedded profile always provides the word class.
func isDeterminerFromDict(s string, dict *profile.Dictionary) bool {
	return dict != nil && dict.HasWordClass(s, wordClassDeterminer)
}

func init() { Register(nounStackChecker{}) }
