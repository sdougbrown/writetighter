package check

import (
	"fmt"
	"sort"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// timeAnchorPhrases are time-anchoring words and phrases, derived from the
// Google developer documentation style guide's timeless-documentation and
// voice-and-tone guidance. Such language ties the document to the moment it
// was written: it reads as current today and stale tomorrow, and it assumes
// the reader knows what changed from a previous version. Feature-announcement
// collocations ("now supports", "is now available") are included because they
// project a change in capability rather than describing how the product
// works.
//
// Deliberately excluded as too imprecise for deterministic linting:
// "new"/"old" (legitimate in "create a new instance"), "latest" (stable
// pointer in "the latest release"), "soon" (legitimate in procedures),
// "existing" (technical, not temporal), and standalone "now" (common
// procedural connective).
var timeAnchorPhrases = []string{
	"as of this writing",
	"as of today",
	"as of now",
	"at the time of writing",
	"at this time",
	"at present",
	"currently",
	"presently",
	"recently",
	"eventually",
	"in the future",
	"in the near future",
	"does not yet",
	"do not yet",
	"doesn't yet",
	"don't yet",
	"coming soon",
	"going forward",
	"now supports",
	"now includes",
	"now lets",
	"now enables",
	"now can",
	"is now available",
	"is now supported",
	"is now enabled",
	"is now deprecated",
	"is now required",
	"is now optional",
}

// timeAnchorOrder is the phrase list sorted longest-first so that "in the
// near future" is reported before, and therefore suppresses, "in the future".
var timeAnchorOrder = func() []string {
	phrases := append([]string(nil), timeAnchorPhrases...)
	sort.Slice(phrases, func(i, j int) bool { return len(phrases[i]) > len(phrases[j]) })
	return phrases
}()

// timeAnchorMessage notes the time-stamped content exception: release notes,
// changelogs, and incident reports are written about a point in time, where
// such language is appropriate.
const timeAnchorMessage = "Time-anchored language goes stale; state the current behavior instead. Exception: time-stamped content such as release notes and changelogs."

type timeAnchorChecker struct{}

func (timeAnchorChecker) ID() string   { return "CORE.TIME_ANCHOR" }
func (timeAnchorChecker) Version() int { return 1 }

func (timeAnchorChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	enforcement, severity := ruleEnforcement(ctx, timeAnchorChecker{}.ID())

	var out []report.Finding
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, phrase := range timeAnchorOrder {
			for _, m := range insensitiveMatches(seg.Text, phrase) {
				start, end := m[0], m[1]
				// A shorter phrase matched inside a longer one (e.g. "in the
				// future" inside "in the near future") is one finding, not two.
				if containedIn(out, ctx.Document.Source, seg, start, end) {
					continue
				}
				path := ctx.Document.Source
				word := seg.Text[start:end]
				out = append(out, report.Finding{
					RuleID:         timeAnchorChecker{}.ID(),
					RuleVersion:    1,
					Checker:        timeAnchorChecker{}.ID(),
					CheckerVersion: 1,
					Enforcement:    enforcement,
					Severity:       severity,
					Path:           &path,
					Range: &report.FindingRange{
						StartByte:   seg.Range.Start.Byte + start,
						EndByte:     seg.Range.Start.Byte + end,
						StartLine:   seg.Range.Start.Line,
						StartColumn: codePointColumn(seg.Text, start, seg.Range.Start.Column),
						EndLine:     seg.Range.Start.Line,
						EndColumn:   codePointColumn(seg.Text, end, seg.Range.Start.Column),
					},
					Evidence:   fmt.Sprintf("time anchor: %q", word),
					Message:    timeAnchorMessage,
					Confidence: 1,
				})
			}
		}
	}
	return out, nil
}

// containedIn reports whether the candidate range [start, end) within the
// given segment is fully covered by an existing finding.
func containedIn(out []report.Finding, source string, seg document.Segment, start, end int) bool {
	absStart := seg.Range.Start.Byte + start
	absEnd := seg.Range.Start.Byte + end
	for _, f := range out {
		if f.Path == nil || *f.Path != source || f.Range == nil {
			continue
		}
		if f.Range.StartByte <= absStart && absEnd <= f.Range.EndByte {
			return true
		}
	}
	return false
}

func init() { Register(timeAnchorChecker{}) }
