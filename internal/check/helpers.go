package check

import (
	"strings"
	"unicode"
)

func lower(s string) string { return strings.ToLower(s) }

// ruleEnforcement returns the enforcement and severity configured for the
// given rule ID in the profile, falling back to "candidate"/"info".
func ruleEnforcement(ctx *RunContext, ruleID string) (enforcement, severity string) {
	enforcement, severity = "candidate", "info"
	if ctx != nil && ctx.Profile != nil && ctx.Profile.Rules != nil {
		for _, rule := range ctx.Profile.Rules.Rules {
			if rule.ID != ruleID {
				continue
			}
			if rule.Enforcement != "" {
				enforcement = rule.Enforcement
			}
			if rule.Severity != "" {
				severity = rule.Severity
			}
			break
		}
	}
	return enforcement, severity
}

func isConjunctionWord(s string) bool {
	s = strings.Trim(s, "()[]{}.,;:!?'\"")
	s = strings.ToLower(s)
	return s == "and" || s == "or"
}

func canonicalMatch(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || r == '-' {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return strings.TrimSpace(b.String())
}

// insensitiveMatches returns byte offsets and only matches complete lexical tokens.
// Offsets are derived from the original string, so UTF-8 source locations stay exact.
func insensitiveMatches(text, term string) [][2]int {
	textRunes, termRunes := []rune(text), []rune(strings.ToLower(term))
	if len(termRunes) == 0 {
		return nil
	}
	offsets := make([]int, 0, len(textRunes)+1)
	for off := range text {
		offsets = append(offsets, off)
	}
	offsets = append(offsets, len(text))
	var out [][2]int
	for i := 0; i+len(termRunes) <= len(textRunes); i++ {
		if i > 0 && (unicode.IsLetter(textRunes[i-1]) || unicode.IsDigit(textRunes[i-1])) {
			continue
		}
		end := i + len(termRunes)
		if end < len(textRunes) && (unicode.IsLetter(textRunes[end]) || unicode.IsDigit(textRunes[end])) {
			continue
		}
		ok := true
		for j := range termRunes {
			if []rune(strings.ToLower(string(textRunes[i+j])))[0] != termRunes[j] {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, [2]int{offsets[i], offsets[end]})
			i = end - 1
		}
	}
	return out
}

func codePointColumn(segment string, byteOffset, initial int) int {
	return initial + len([]rune(segment[:byteOffset]))
}

// isHeadingMarker reports whether marker is an ATX heading run ("#" through
// "######"). Heading blocks from document.AnalyzeProse carry exactly this
// marker; the run length is the heading level.
func isHeadingMarker(marker string) bool {
	if marker == "" {
		return false
	}
	for i := 0; i < len(marker); i++ {
		if marker[i] != '#' {
			return false
		}
	}
	return true
}

func headingLevel(marker string) int { return len(marker) }

// isUnorderedItemMarker reports whether marker is a bulleted list item marker
// ("- ", "* ", "+ ").
func isUnorderedItemMarker(marker string) bool {
	return marker == "- " || marker == "* " || marker == "+ "
}

// isOrderedItemMarker reports whether marker is a numbered list item marker
// ("1.", "12)").
func isOrderedItemMarker(marker string) bool {
	if len(marker) < 2 {
		return false
	}
	last := marker[len(marker)-1]
	if last != '.' && last != ')' {
		return false
	}
	for i := 0; i < len(marker)-1; i++ {
		if marker[i] < '0' || marker[i] > '9' {
			return false
		}
	}
	return true
}

// headingStopwords are the function words that carry no information about a
// heading's casing: title case and sentence case treat them identically.
var headingStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true,
	"but": true, "nor": true, "for": true, "as": true, "at": true,
	"by": true, "in": true, "of": true, "on": true, "to": true,
	"with": true, "vs": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "it": true, "its": true,
}

// headingWords splits heading text into whitespace-delimited words containing
// at least one letter, with surrounding punctuation stripped.
func headingWords(text string) []string {
	var words []string
	for _, raw := range strings.Fields(text) {
		word := strings.Trim(raw, ",.;:!?\"'`()[]{}\t ")
		if word == "" {
			continue
		}
		for _, r := range word {
			if unicode.IsLetter(r) {
				words = append(words, word)
				break
			}
		}
	}
	return words
}

// isCapitalized reports whether the word's first letter rune is uppercase.
func isCapitalized(word string) bool {
	for _, r := range word {
		if unicode.IsLetter(r) {
			return unicode.IsUpper(r)
		}
	}
	return false
}
