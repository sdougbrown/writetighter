package check

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// countFragmentChecker flags an adjacent pair of count-led fragments:
//
//	"Six legs. One GPU, one server build, one sampler, one question set."
//
// The first sentence is a short count-led fragment with no finite verb; the
// second is an inventory of comma-separated count-led phrases. Both must be
// present for a finding — an isolated inventory list is ordinary prose.
//
// The finding describes the cadence only; whether the staccato rhythm fits
// the surrounding prose is a judgment for the reader, so the rule ships as
// candidate/info.
type countFragmentChecker struct{}

func (countFragmentChecker) ID() string   { return "CORE.COUNT_FRAGMENT" }
func (countFragmentChecker) Version() int { return 1 }

const (
	// countFragmentMaxSetupWords caps the first sentence ("Six legs.").
	countFragmentMaxSetupWords = 6
	// countFragmentMaxInventoryWords caps the inventory sentence.
	countFragmentMaxInventoryWords = 15
	// countFragmentMinSegments is the minimum number of comma-separated
	// count-led phrases required in the inventory sentence. The canonical
	// example has four segments separated by three commas.
	countFragmentMinSegments = 3
)

// countFragmentAuxiliaries are the auxiliary/copular words that mark a
// complete clause. Combined with the finite_verb negative evidence this keeps
// "Six legs were tested. One GPU served all requests." (complete clauses)
// out of the template.
var countFragmentAuxiliaries = map[string]bool{
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "am": true,
	"have": true, "has": true, "had": true,
	"will": true, "would": true, "can": true, "could": true,
	"should": true, "shall": true, "may": true, "might": true, "must": true,
	"do": true, "does": true, "did": true,
}

func (countFragmentChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}

	// The rule needs the cardinal_number word class; a profile without it
	// means the checker abstains (graceful degradation), never fires blind.
	var cardinals map[string]bool
	if ctx.Profile != nil && ctx.Profile.Dict != nil {
		cardinals = ctx.Profile.Dict.WordClassSet("cardinal_number")
	}
	if len(cardinals) == 0 {
		return nil, nil
	}
	var finiteVerbs, partExc map[string]bool
	if ctx.Profile != nil && ctx.Profile.Dict != nil {
		finiteVerbs = ctx.Profile.Dict.WordClassSet("finite_verb")
		partExc = ctx.Profile.Dict.WordClassSet("participle_head_exception")
	}

	enforcement, severity := ruleEnforcement(ctx, countFragmentChecker{}.ID())

	blocks := document.AnalyzeProse(ctx.Document)
	var out []report.Finding
	for _, block := range blocks {
		// Skip list items, headings, and blockquotes: enumerated inventories
		// are legitimate in those shapes.
		if block.Marker != "" {
			continue
		}
		sentences := document.SentenceUnits(block, ctx.Document.Content)
		for i := 0; i+1 < len(sentences); i++ {
			s1, s2 := sentences[i], sentences[i+1]
			if !isCountLedFragment(s1.Text, s1.WordCount, cardinals, finiteVerbs, partExc) {
				continue
			}
			if !isCountLedInventory(s2.Text, s2.WordCount, cardinals) {
				continue
			}
			// Quotes are unmarked by the sentence splitter; abstain.
			if containsQuotePunctuation(s1.Text) || containsQuotePunctuation(s2.Text) {
				continue
			}
			// Table rows survive AnalyzeProse without a marker; a pipe is
			// the cheapest reliable tell.
			if strings.ContainsRune(s1.Text, '|') || strings.ContainsRune(s2.Text, '|') {
				continue
			}
			path := ctx.Document.Source
			rng := &report.FindingRange{
				StartByte: s1.StartByte, EndByte: s2.EndByte,
				StartLine: s1.StartLine, StartColumn: s1.StartColumn,
				EndLine: s2.EndLine, EndColumn: s2.EndColumn,
			}
			out = append(out, report.Finding{
				RuleID:         countFragmentChecker{}.ID(),
				RuleVersion:    1,
				Checker:        countFragmentChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range:          rng,
				Evidence:       fmt.Sprintf("count-led fragment pair: %q / %q", s1.Text, s2.Text),
				Message:        "Repeated count-led fragments. Consider a sentence or list if these describe one setup.",
				Confidence:     1,
			})
			i++ // the pair consumes both sentences; do not chain findings
		}
	}
	return out, nil
}

// isCountLedFragment reports whether text is a short count-led fragment with
// no verb: "Six legs.", "Three constraints.".
func isCountLedFragment(text string, wordCount int, cardinals, finiteVerbs, partExc map[string]bool) bool {
	if wordCount < 2 || wordCount > countFragmentMaxSetupWords {
		return false
	}
	tokens := document.ExportLexicalTokens(text)
	if !isCardinalToken(tokens[0], cardinals) {
		return false
	}
	// Negative evidence only: a known finite verb, auxiliary, or past-
	// participle-looking token means this is a complete clause, not a
	// fragment. Absence from the lists proves nothing, so the exclusions
	// sacrifice recall rather than claim grammatical certainty.
	for _, tok := range tokens[1:] {
		l := lower(tok)
		if finiteVerbs[l] || countFragmentAuxiliaries[l] {
			return false
		}
		if len(l) >= 4 && strings.HasSuffix(l, "ed") && !partExc[l] {
			return false
		}
	}
	return true
}

// isCountLedInventory reports whether text is a comma-separated inventory of
// count-led phrases: "One GPU, one server build, one sampler, one question set.".
func isCountLedInventory(text string, wordCount int, cardinals map[string]bool) bool {
	if wordCount > countFragmentMaxInventoryWords {
		return false
	}
	tokens := document.ExportLexicalTokens(text)
	if !isCardinalToken(tokens[0], cardinals) {
		return false
	}
	segments := strings.Split(text, ",")
	if len(segments) < countFragmentMinSegments {
		return false
	}
	for _, seg := range segments {
		segTokens := document.ExportLexicalTokens(seg)
		if len(segTokens) == 0 || !isCardinalToken(segTokens[0], cardinals) {
			return false
		}
	}
	return true
}

// isCardinalToken reports whether tok is a cardinal number: a digit run or a
// word from the cardinal_number class. Digits are recognized in code so the
// class only has to carry spelled-out numbers.
func isCardinalToken(tok string, cardinals map[string]bool) bool {
	if tok == "" {
		return false
	}
	for _, r := range tok {
		if !unicode.IsDigit(r) {
			return cardinals[lower(tok)]
		}
	}
	return true
}

func init() { Register(countFragmentChecker{}) }
