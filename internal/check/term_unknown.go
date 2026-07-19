package check

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

type termUnknownChecker struct{}

func (termUnknownChecker) ID() string   { return "CORE.TERM_UNKNOWN" }
func (termUnknownChecker) Version() int { return 1 }

// knownTerms returns dictionary entries sorted by folded length descending
// for longest-phrase matching.
func knownTerms(dict *profile.Dictionary) []profile.Entry {
	if dict == nil {
		return nil
	}
	terms := make([]profile.Entry, 0, len(dict.Entries))
	for _, e := range dict.Entries {
		terms = append(terms, e)
	}
	sort.Slice(terms, func(i, j int) bool {
		return len(folded(terms[i].Term)) > len(folded(terms[j].Term))
	})
	return terms
}

func folded(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// matchPhrase checks if any phrase from the text at position start matches
// a dictionary entry using longest-first matching. Returns the matched entry
// and the end position (in runes) of the match.
func matchPhrase(text string, start int, terms []profile.Entry) (*profile.Entry, int) {
	if start >= len(text) {
		return nil, start
	}
	// Try each known term from longest to shortest
	for _, te := range terms {
		fTerm := folded(te.Term)
		if len(fTerm) == 0 {
			continue
		}
		end := start + len(fTerm)
		if end > len(text) {
			continue
		}
		seg := text[start:end]
		if folded(seg) == fTerm {
			return &te, end
		}
	}
	return nil, start
}

func (termUnknownChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.Profile == nil || ctx.Profile.Rules == nil || ctx.Profile.Rules.UnknownTermPolicy != "candidate" || ctx.Profile.Dict == nil {
		return nil, nil
	}
	var out []report.Finding
	known := knownTerms(ctx.Profile.Dict)

	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		text := seg.Text
		runes := []rune(text)

		// Build byte-offset map: rune index -> byte offset in segment
		runeToByte := make([]int, len(runes)+1)
		runeToByte[0] = 0
		for i := 0; i < len(runes); i++ {
			runeToByte[i+1] = runeToByte[i] + len(string(runes[i]))
		}

		// Track which rune positions have been consumed by multi-word matches
		consumed := make(map[int]bool)

		// First pass: find all multi-word phrase matches (longest first)
		pos := 0
		for pos < len(runes) {
			if unicode.IsSpace(runes[pos]) {
				pos++
				continue
			}
			matchedEntry, matchEnd := matchPhrase(text[pos:], 0, known)
			if matchedEntry != nil {
				// Mark all rune positions in this span as consumed
				for i := pos; i < matchEnd && i < len(runes); i++ {
					consumed[i] = true
				}
				pos = matchEnd
				continue
			}
			pos++
		}

		// Second pass: report unknowns for non-consumed tokens
		pos = 0
		for pos < len(runes) {
			if unicode.IsSpace(runes[pos]) {
				pos++
				continue
			}
			if consumed[pos] {
				pos++
				continue
			}

			// Find the extent of this token (whitespace-delimited)
			tokenStart := pos
			for pos < len(runes) && !unicode.IsSpace(runes[pos]) {
				pos++
			}
			tokenEnd := pos

			// Check if any part of this token was consumed by a phrase match
			allConsumed := true
			for i := tokenStart; i < tokenEnd; i++ {
				if !consumed[i] {
					allConsumed = false
					break
				}
			}
			if allConsumed {
				continue
			}

			token := string(runes[tokenStart:tokenEnd])

			// Clean: trim non-letter/non-digit characters from edges
			clean := strings.TrimFunc(token, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
			if clean == "" {
				continue
			}

			if ctx.Profile.Dict.Lookup(clean) != nil {
				continue
			}

			byteStart := runeToByte[tokenStart]
			byteEnd := runeToByte[tokenEnd]

			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         termUnknownChecker{}.ID(),
				RuleVersion:    1,
				Checker:        termUnknownChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    "candidate",
				Severity:       "info",
				Path:           &path,
				Range: &report.FindingRange{
					StartByte:   seg.Range.Start.Byte + byteStart,
					EndByte:     seg.Range.Start.Byte + byteEnd,
					StartLine:   seg.Range.Start.Line,
					StartColumn: seg.Range.Start.Column + byteStart,
					EndLine:     seg.Range.End.Line,
					EndColumn:   seg.Range.End.Column + byteEnd,
				},
				Evidence:   fmt.Sprintf("Unknown term: %s", clean),
				Message:    fmt.Sprintf("Unknown term: %s", clean),
				Confidence: 1,
			})
		}
	}
	return out, nil
}
func init() { Register(termUnknownChecker{}) }
