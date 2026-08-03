package llm

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"
)

// Excerpt represents a bounded portion of a document sent to the LLM.
type Excerpt struct {
	// Text is the excerpt text sent to the LLM. Byte offsets in the
	// LLM response are relative to this.
	Text string
	// OrigOffset maps a byte offset in Text to the corresponding
	// byte offset in the original document content. If the excerpt
	// is the full document content, OrigOffset is identity (i = i).
	OrigOffset func(excerptPos int) int
	// origOffsets is the per-byte mapping from excerpt to original offsets.
	// It is stored so that OrigToExcerpt can invert it and so that
	// cross-gap validation can inspect the mapping.
	origOffsets []int
	// analysisStart is the virtual HTML offset, not a raw-source coordinate.
	analysisStart int
	virtual       bool
}

// validExcerptRange reports whether an excerpt-relative byte range [start, end)
// is valid: both positions map to real (non-separator) original content,
// the range does not cross a synthetic separator, and the original content
// is contiguous (no gaps across different segments).
// The end offset is exclusive; it must map to the byte immediately after
// the last real content byte in the excerpt (which is origOffsets[end-1]+1
// or len(doc.Content)).
func (e *Excerpt) validExcerptRange(start, end int) bool {
	if start < 0 || end < start || end > len(e.Text) {
		return false
	}
	if start == end {
		return true
	}
	if e.virtual {
		return true
	}
	// Both start and end-1 must be real (non-separator) positions.
	if e.origOffsets == nil {
		return true // identity mapping; always valid
	}
	if start >= len(e.origOffsets) || end-1 >= len(e.origOffsets) {
		return false
	}
	if e.origOffsets[start] < 0 || e.origOffsets[end-1] < 0 {
		return false // range begins or ends at a separator
	}
	// The original range must be contiguous: the distance from start to
	// end-1 in the original must be end-1-start in the excerpt,
	// and there must be no separator in between.
	expectedOrig := e.origOffsets[start]
	for i := start; i < end; i++ {
		if e.origOffsets[i] < 0 {
			return false // separator inside range
		}
		if e.origOffsets[i] != expectedOrig+(i-start) {
			return false // noncontiguous
		}
	}
	return true
}

// exclusiveOrigEnd returns the exclusive end byte offset in the original
// document that corresponds to len(e.Text). For a contiguous excerpt built
// from original segments, this is origOffsets[len(origOffsets)-1]+1 when
// the last byte is real, or len(doc.Content) as a fallback.
func (e *Excerpt) exclusiveOrigEnd() int {
	if len(e.origOffsets) == 0 {
		return 0
	}
	last := e.origOffsets[len(e.origOffsets)-1]
	if last >= 0 {
		return last + 1
	}
	// Last position is a separator; find the last real offset.
	for i := len(e.origOffsets) - 2; i >= 0; i-- {
		if e.origOffsets[i] >= 0 {
			return e.origOffsets[i] + 1
		}
	}
	return 0
}

// OrigToExcerpt maps an original-document byte offset to an excerpt byte offset.
// Returns -1 if the original offset is not present in the excerpt.
func (e *Excerpt) OrigToExcerpt(origPos int) int {
	if e.virtual {
		return -1
	}
	if e.origOffsets == nil {
		if origPos >= 0 && origPos <= len(e.Text) {
			return origPos
		}
		return -1
	}
	for i, o := range e.origOffsets {
		if o == origPos {
			return i
		}
	}
	// An exclusive original end may sit immediately after the last byte of
	// an included segment and therefore have no byte entry of its own.
	for i, o := range e.origOffsets {
		if o >= 0 && o+1 == origPos && (i+1 == len(e.origOffsets) || e.origOffsets[i+1] != origPos) {
			return i + 1
		}
	}
	return -1
}

// NewChunkExcerpt creates an Excerpt for a contiguous byte range of a document.
// It is used externally (e.g., by app package) for budget validation during
// chunk planning.
func NewChunkExcerpt(doc *document.Document, start, end int) *Excerpt {
	if doc.Format == document.FormatHTML {
		return newVirtualExcerpt(doc.AnalysisContent()[start:end], start)
	}
	offsets := make([]int, end-start)
	for i := range offsets {
		offsets[i] = start + i
	}
	return newExcerpt(doc.Content[start:end], offsets)
}

func newExcerpt(text string, offsets []int) *Excerpt {
	e := &Excerpt{Text: text, origOffsets: offsets}
	e.OrigOffset = e.originalOffset
	return e
}

func newVirtualExcerpt(text string, start int) *Excerpt {
	e := &Excerpt{Text: text, analysisStart: start, virtual: true}
	e.OrigOffset = func(int) int { return -1 }
	return e
}

func (e *Excerpt) originalOffset(pos int) int {
	if pos < 0 || pos > len(e.Text) {
		return -1
	}
	if e.origOffsets == nil {
		return pos
	}
	if pos == len(e.origOffsets) {
		return exclusiveOrigEndFromOffsets(e.origOffsets)
	}
	if e.origOffsets[pos] >= 0 {
		return e.origOffsets[pos]
	}
	// A valid exclusive end can point at a synthetic separator. Map it to
	// one byte past the preceding real byte; valid range starts never point
	// at separators.
	for i := pos - 1; i >= 0; i-- {
		if e.origOffsets[i] >= 0 {
			return e.origOffsets[i] + 1
		}
	}
	return -1
}

// truncateExcerpt returns a UTF-8-safe prefix while preserving offset mapping.
func truncateExcerpt(e *Excerpt, n int) *Excerpt {
	if n <= 0 || len(e.Text) <= n {
		return e
	}
	// Find the largest valid UTF-8 prefix within n bytes.
	nValid := utf8ValidPrefixLen(e.Text, n)
	newText := e.Text[:nValid]
	if e.virtual {
		return newVirtualExcerpt(newText, e.analysisStart)
	}
	// Also truncate origOffsets if present.
	var newOffsets []int
	if e.origOffsets != nil {
		if nValid <= len(e.origOffsets) {
			newOffsets = e.origOffsets[:nValid]
		} else {
			newOffsets = e.origOffsets
		}
	}
	return newExcerpt(newText, newOffsets)
}

// utf8ValidPrefixLen returns the largest n <= max where text[:n] is valid UTF-8.
func utf8ValidPrefixLen(text string, max int) int {
	if max <= 0 {
		return 0
	}
	if max > len(text) {
		max = len(text)
	}
	// If text[:max] is already valid UTF-8 and we're at a rune boundary, return max.
	if utf8.ValidString(text[:max]) && (max == 0 || max == len(text) || utf8.RuneStart(text[max])) {
		return max
	}
	// Walk backwards until we find valid UTF-8 ending at a boundary.
	for max > 0 {
		// Check if text[:max] is valid UTF-8.
		if utf8.Valid([]byte(text[:max])) {
			// Also ensure max is at a rune start (or end of string).
			if max == len(text) || utf8.RuneStart(text[max]) {
				return max
			}
		}
		max--
	}
	return 0
}

func exclusiveOrigEndFromOffsets(origOffsets []int) int {
	if len(origOffsets) == 0 {
		return 0
	}
	last := origOffsets[len(origOffsets)-1]
	if last >= 0 {
		return last + 1
	}
	for i := len(origOffsets) - 2; i >= 0; i-- {
		if origOffsets[i] >= 0 {
			return origOffsets[i] + 1
		}
	}
	return 0
}

// termOccurs reports whole-term, case-insensitive occurrence.
func termOccurs(text, term string) bool {
	if term == "" || len(term) > len(text) {
		return false
	}
	text = strings.ToLower(text)
	term = strings.ToLower(term)
	for offset := 0; offset <= len(text)-len(term); {
		relative := strings.Index(text[offset:], term)
		if relative < 0 {
			return false
		}
		start := offset + relative
		end := start + len(term)
		beforeWord := false
		if start > 0 {
			r, _ := utf8.DecodeLastRuneInString(text[:start])
			beforeWord = unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
		}
		afterWord := false
		if end < len(text) {
			r, _ := utf8.DecodeRuneInString(text[end:])
			afterWord = unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
		}
		if !beforeWord && !afterWord {
			return true
		}
		offset = start + 1
	}
	return false
}
