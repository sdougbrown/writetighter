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
