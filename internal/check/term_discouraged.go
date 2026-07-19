package check

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

type termDiscouragedChecker struct{}

func (termDiscouragedChecker) ID() string   { return "CORE.TERM_DISCOURAGED" }
func (termDiscouragedChecker) Version() int { return 1 }

// hasWordBoundary reports whether position pos in s satisfies word-boundary context.
// atEnd=true means pos is right after the match end; atEnd=false means pos is the match start.
func hasWordBoundary(s string, pos int, atEnd bool) bool {
	if atEnd {
		if pos >= len(s) {
			return true
		}
		r, _ := utf8.DecodeRuneInString(s[pos:])
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}
	if pos <= 0 {
		return true
	}
	_, size := utf8.DecodeLastRuneInString(s[:pos])
	r := rune(s[pos-1])
	_ = size
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func (termDiscouragedChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil || ctx.Profile == nil || ctx.Profile.Dict == nil {
		return nil, nil
	}
	var out []report.Finding

	// Collect and sort discouraged entries by term byte length (longest first).
	var terms []profile.Entry
	for _, entry := range ctx.Profile.Dict.Entries {
		if entry.Status == profile.StatusDiscouraged {
			terms = append(terms, entry)
		}
	}
	sort.Slice(terms, func(i, j int) bool {
		return len(terms[i].Term) > len(terms[j].Term)
	})

	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		lowerText := strings.ToLower(seg.Text)

		// Track covered byte positions to avoid overlapping matches.
		covered := make([]bool, len(lowerText))

		for _, entry := range terms {
			needle := strings.ToLower(entry.Term)
			start := 0
			for {
				pos := strings.Index(lowerText[start:], needle)
				if pos < 0 {
					break
				}
				actualPos := start + pos
				matchEnd := actualPos + len(needle)

				// Check word boundaries.
				if !hasWordBoundary(lowerText, actualPos, false) ||
					!hasWordBoundary(lowerText, matchEnd, true) {
					start = actualPos + 1
					continue
				}

				// Check if any position in this match range is already covered.
				skip := false
				for i := actualPos; i < matchEnd; i++ {
					if covered[i] {
						skip = true
						break
					}
				}
				if skip {
					start = actualPos + 1
					continue
				}

				// Mark all positions as covered.
				for i := actualPos; i < matchEnd; i++ {
					covered[i] = true
				}

				path := ctx.Document.Source
				suggestion := strings.Join(entry.Alternatives, ", ")
				out = append(out, report.Finding{
					RuleID:         termDiscouragedChecker{}.ID(),
					RuleVersion:    1,
					Checker:        termDiscouragedChecker{}.ID(),
					CheckerVersion: 1,
					Enforcement:    "enforced",
					Severity:       "warning",
					Path:           &path,
					Range: &report.FindingRange{
						StartByte:   seg.Range.Start.Byte + actualPos,
						EndByte:     seg.Range.Start.Byte + matchEnd,
						StartLine:   seg.Range.Start.Line,
						StartColumn: seg.Range.Start.Column + actualPos,
						EndLine:     seg.Range.Start.Line,
						EndColumn:   seg.Range.Start.Column + matchEnd,
					},
					Evidence:   fmt.Sprintf("'%s' is discouraged; use '%s' instead", entry.Term, suggestion),
					Message:    entry.Reason,
					Suggestion: &suggestion,
					Confidence: 1,
				})
				start = matchEnd
			}
		}
	}
	return out, nil
}
func init() { Register(termDiscouragedChecker{}) }
