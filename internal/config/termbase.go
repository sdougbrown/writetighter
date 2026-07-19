package config

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func validateTermBase(entries []TermEntry) error {
	seen := map[string]struct{}{}
	for i, entry := range entries {
		term := strings.TrimSpace(entry.Term)
		if term == "" {
			return fmt.Errorf("term %d: empty term", i)
		}
		if entry.Override && strings.TrimSpace(entry.Reason) == "" {
			return fmt.Errorf("term %q: override requires reason", entry.Term)
		}
		key := foldString(term)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate term %q", entry.Term)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func foldString(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		b.WriteRune(unicode.ToLower(r))
		s = s[size:]
	}
	return b.String()
}
