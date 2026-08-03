package check

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
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
	"about": true, "as": true, "than": true, "per": true,
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
			tokens := document.ExportLexicalTokens(s.Text)
			if len(tokens) < threshold {
				continue
			}

			// Find maximal runs of consecutive content (non-stopword) tokens.
			runStart := -1
			for i := 0; i <= len(tokens); i++ {
				if i < len(tokens) && !proseStopwords[strings.ToLower(tokens[i])] {
					if runStart < 0 {
						runStart = i
					}
					continue
				}
				// End of a content run (hit a stopword or end of tokens).
				if runStart < 0 {
					continue
				}
				runLen := i - runStart
				if runLen >= threshold {
					stack := strings.Join(tokens[runStart:i], " ")
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
						Evidence:       fmt.Sprintf("noun stack (%d content words): %q", runLen, stack),
						Message:        "Long noun stack. Consider unpacking into subject-verb-object.",
						Confidence:     1,
					})
				}
				runStart = -1
			}
		}
	}
	return out, nil
}

func init() { Register(nounStackChecker{}) }
