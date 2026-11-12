package check

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// negationPairChecker flags a negative copular setup sentence followed by an
// adjacent positive restatement with the same pronoun subject:
//
//	"It is not a wrong answer. It is a non-answer."
//
// The finding describes the observable structure only. Whether the contrast
// is a legitimate technical distinction ("It is not a mutex. It is a
// semaphore.") is a judgment for the reader, so the rule ships as
// candidate/info and the message never claims the prose must change.
type negationPairChecker struct{}

func (negationPairChecker) ID() string   { return "CORE.NEGATION_PAIR" }
func (negationPairChecker) Version() int { return 1 }

// negationPairSubjects are the pronoun subjects the template allows.
var negationPairSubjects = map[string]bool{"it": true, "this": true, "that": true}

// negationPairCopulas are the copular verbs the template requires.
var negationPairCopulas = map[string]bool{"is": true, "are": true, "was": true, "were": true}

// negationPairDeterminers are the determiners allowed before the complement
// noun in both sentences. Contractions ("it isn't") are deliberately outside
// the template: CORE.CONTRACTION already flags them.
var negationPairDeterminers = map[string]bool{"a": true, "an": true, "the": true}

// negationPairMaxWords caps each sentence of the pair. Long sentences that
// happen to open with the template are usually substantive arguments, not
// the tic.
const negationPairMaxWords = 12

func (negationPairChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}

	enforcement, severity := ruleEnforcement(ctx, negationPairChecker{}.ID())

	blocks := document.AnalyzeProse(ctx.Document)
	var out []report.Finding
	for _, block := range blocks {
		// Skip marked blocks (list items, headings, blockquotes): the tic is a
		// prose move, and quoted or enumerated contrasts are legitimate. The
		// exclusion keys on Markdown markers only — HTML blockquote/li/td text
		// reaches the checker markerless, like in the sibling checkers.
		if block.Marker != "" {
			continue
		}
		sentences := document.SentenceUnits(block, ctx.Document.Content)
		for i := 0; i+1 < len(sentences); i++ {
			s1, s2 := sentences[i], sentences[i+1]
			if !isNegationSetup(s1.Text) || !isPositiveRestatement(s2.Text) {
				continue
			}
			// The tic restates with the same pronoun; different pronouns read
			// as two separate observations, not a setup/restatement pair.
			if negationPairSubject(s1.Text) != negationPairSubject(s2.Text) {
				continue
			}
			// Quotes are not marked by the sentence splitter, so any
			// quotation punctuation means we cannot tell quoted speech
			// from author prose — abstain.
			if containsQuotePunctuation(s1.Text) || containsQuotePunctuation(s2.Text) {
				continue
			}
			// Table rows survive AnalyzeProse without a marker; a pipe is
			// the cheapest reliable tell.
			if strings.ContainsRune(s1.Text, '|') || strings.ContainsRune(s2.Text, '|') {
				continue
			}
			if s1.WordCount > negationPairMaxWords || s2.WordCount > negationPairMaxWords {
				continue
			}
			path := ctx.Document.Source
			rng := &report.FindingRange{
				StartByte: s1.StartByte, EndByte: s2.EndByte,
				StartLine: s1.StartLine, StartColumn: s1.StartColumn,
				EndLine: s2.EndLine, EndColumn: s2.EndColumn,
			}
			out = append(out, report.Finding{
				RuleID:         negationPairChecker{}.ID(),
				RuleVersion:    1,
				Checker:        negationPairChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range:          rng,
				Evidence:       fmt.Sprintf("negation-first pair: %q / %q", s1.Text, s2.Text),
				Message:        "Negative setup followed by a positive restatement. Consider leading with the distinction.",
				Confidence:     1,
			})
			i++ // the pair consumes both sentences; do not chain findings
		}
	}
	return out, nil
}

// negationPairSubject reports the lowercased pronoun subject the text opens
// with, or "" when it opens with something else. Both template helpers already
// reject non-subject openers, so callers use this only to compare the pair.
func negationPairSubject(text string) string {
	tokens := document.ExportLexicalTokens(text)
	if len(tokens) == 0 || !negationPairSubjects[lower(tokens[0])] {
		return ""
	}
	return lower(tokens[0])
}

// isNegationSetup reports whether text matches the setup template:
// pronoun + copula + "not" + determiner + at least one content word.
func isNegationSetup(text string) bool {
	tokens := document.ExportLexicalTokens(text)
	if len(tokens) < 5 {
		return false
	}
	if !negationPairSubjects[lower(tokens[0])] || !negationPairCopulas[lower(tokens[1])] {
		return false
	}
	if lower(tokens[2]) != "not" || !negationPairDeterminers[lower(tokens[3])] {
		return false
	}
	// The complement must be noun-like content, not another function word
	// ("It is not a way to..." is a hedge, not the reveal).
	return !isNegationPairFunctionWord(lower(tokens[4]))
}

// isPositiveRestatement reports whether text matches the restatement template:
// pronoun + copula + determiner + at least one content word.
func isPositiveRestatement(text string) bool {
	tokens := document.ExportLexicalTokens(text)
	if len(tokens) < 4 {
		return false
	}
	if !negationPairSubjects[lower(tokens[0])] || !negationPairCopulas[lower(tokens[1])] {
		return false
	}
	if !negationPairDeterminers[lower(tokens[2])] {
		return false
	}
	return !isNegationPairFunctionWord(lower(tokens[3]))
}

// isNegationPairFunctionWord reports whether tok is a pronoun, determiner, or
// preposition — words that cannot carry the reveal noun the template needs.
// Deliberately small: the goal is to exclude "It is not one of them. It is
// the only one."-shaped hedges without a full part-of-speech model.
var negationPairFunctionWords = map[string]bool{
	"it": true, "this": true, "that": true, "these": true, "those": true,
	"one": true, "another": true, "other": true, "some": true, "any": true,
	"all": true, "each": true, "both": true, "either": true, "neither": true,
	"the": true, "a": true, "an": true, "of": true, "for": true, "to": true,
	"with": true, "by": true, "from": true, "in": true, "on": true, "at": true,
}

func isNegationPairFunctionWord(tok string) bool { return negationPairFunctionWords[tok] }

// containsQuotePunctuation reports whether text carries double-quote
// punctuation (straight or curly). Apostrophes are excluded on purpose —
// they are contractions and possessives far more often than quotations.
func containsQuotePunctuation(text string) bool {
	return strings.ContainsAny(text, "\"“”")
}

func init() { Register(negationPairChecker{}) }
