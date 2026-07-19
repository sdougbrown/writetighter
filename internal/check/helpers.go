package check

import (
	"strings"
	"unicode"
)

func lower(s string) string { return strings.ToLower(s) }

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
