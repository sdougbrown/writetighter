package check

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/report"
)

// proseStopwords are function words that do not participate in noun stacks.
// This is a small, stable set — not a growing dictionary of bad phrases.
// Content words (nouns, verbs, adjectives, technical terms) are everything
// not in this set. A run of 3+ consecutive content words is a noun-stack
// candidate.
var proseStopwords = map[string]bool{
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
	// common contractions (auxiliary/modal verbs)
	"isn't": true, "aren't": true, "wasn't": true, "weren't": true,
	"don't": true, "doesn't": true, "didn't": true, "won't": true,
	"wouldn't": true, "can't": true, "couldn't": true, "shouldn't": true,
	"hasn't": true, "haven't": true, "hadn't": true,
	"it's": true, "they're": true, "we're": true, "you're": true,
	"that's": true, "there's": true, "here's": true, "what's": true,
	"who's": true, "let's": true,
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

// nounRunFinalVerbs is a closed, high-signal set of frequent finite verbs that
// commonly close a clause-shaped window. A content run ending in one of these
// is a subject+predicate clause ("unit conversions use", "keys keep"), never a
// noun stack. This is deliberately not a POS tagger; see endsInFinalVerb for
// the accompanying participial guard.
var nounRunFinalVerbs = map[string]bool{
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
	"stand": true, "stands": true, "stood": true,
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
}

// irregularParticiples are strong-verb participle forms that close a reduced
// clause ("class chosen", "path taken"). They only matter at the end of a
// window; the regular -ed forms are handled by the suffix guard.
var irregularParticiples = map[string]bool{
	"taken": true, "given": true, "broken": true, "written": true,
	"spoken": true, "hidden": true, "eaten": true, "seen": true,
	"fallen": true, "driven": true, "frozen": true, "beaten": true,
	"forgotten": true, "shaken": true, "thrown": true, "gone": true,
}

// participle final tokens mark a reduced clause, not a noun phrase: "barcode
// registered", "class chosen", "zeros stripped". -ing is deliberately absent —
// gerunds ("connection pooling") are legitimate noun-stack heads. participleHeadExceptions
// preserves the handful of common content nouns that happen to end in -ed.
func endsInParticiple(token string) bool {
	lower := strings.ToLower(token)
	if irregularParticiples[lower] {
		return true
	}
	return len(lower) >= 4 && strings.HasSuffix(lower, "ed") && !participleHeadExceptions[lower]
}

var participleHeadExceptions = map[string]bool{
	"feed": true, "seed": true, "need": true, "deed": true,
	"speed": true, "breed": true, "weed": true, "reed": true,
	"greed": true, "creed": true, "shed": true, "sled": true,
	"bleed": true, "plead": true, "indeed": true, "succeed": true,
	"exceed": true, "proceed": true,
}

// identifierOrProperNounToken reports a token that unambiguously marks an
// identifier- or proper-name-heavy technical stack in code-adjacent prose:
// mixed-case names (primaryCategoryId, StateWrapper, getNativeModule, iPad),
// tokens carrying digits (Level-1, RN0.82), or a short all-caps acronym
// (EI, NPE). A merely sentence-initial capital ("Client", "Apollo") is
// treated as ordinary prose so plain-language stacks survive; the separate
// CORE.UNEXPANDED_ABBREV rule covers jargon in the acronym itself.
func identifierOrProperNounToken(token string) bool {
	hasDigit := false
	uppercaseCount := 0
	for i, r := range token {
		if unicode.IsDigit(r) {
			hasDigit = true
		}
		if unicode.IsUpper(r) {
			uppercaseCount++
			// An uppercase in a non-initial position marks PascalCase or
			// camelCase, which prose words at sentence start never have.
			if i > 0 {
				return true
			}
		}
	}
	if hasDigit {
		return true
	}
	// Remaining all-uppercase token of acronym length.
	n := utf8.RuneCountInString(token)
	return uppercaseCount >= 2 && uppercaseCount <= 5 && uppercaseCount == n
}

// scanNounStackRuns walks a sentence and returns maximal runs of consecutive
// content (non-stopword) words. Runs are split at clause boundaries, so a
// window never spans an em/en dash, semicolon, or colon.
func scanNounStackRuns(text string) [][]string {
	runes := []rune(text)
	n := len(runes)
	var runs [][]string
	var cur []string
	i := 0
	for i < n {
		r := runes[i]
		switch {
		case isClauseBoundary(r):
			if len(cur) > 0 {
				runs = append(runs, cur)
				cur = nil
			}
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
			if proseStopwords[strings.ToLower(tok)] {
				if len(cur) > 0 {
					runs = append(runs, cur)
					cur = nil
				}
				continue
			}
			cur = append(cur, tok)
		}
	}
	if len(cur) > 0 {
		runs = append(runs, cur)
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
			runs := scanNounStackRuns(s.Text)
			for _, run := range runs {
				if len(run) < threshold {
					continue
				}
				if !isNounPhraseRun(run) {
					continue
				}
				if codeAdjacent && stackHasIdentifier(run) {
					continue
				}
				stack := strings.Join(run, " ")
				path := ctx.Document.Source
				rng := &report.FindingRange{
					StartByte: s.StartByte, EndByte: s.EndByte,
					StartLine: s.StartLine, StartColumn: s.StartColumn,
					EndLine: s.EndLine, EndColumn: s.EndColumn,
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
					Evidence:       fmt.Sprintf("noun stack (%d content words): %q", len(run), stack),
					Message:        "Long noun stack. Consider unpacking into subject-verb-object.",
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
func isNounPhraseRun(run []string) bool {
	for _, tok := range run {
		if nounRunFinalVerbs[lower(tok)] {
			return false
		}
	}
	return !endsInParticiple(run[len(run)-1])
}

func stackHasIdentifier(run []string) bool {
	for _, tok := range run {
		if identifierOrProperNounToken(tok) {
			return true
		}
	}
	return false
}

func init() { Register(nounStackChecker{}) }
