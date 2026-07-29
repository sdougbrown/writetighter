package llm

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
)

var protectedTokenPattern = regexp.MustCompile(`https?://[^\s<>"']+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|\b[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\b|\b(?:\d{1,3}\.){3}\d{1,3}\b|(?:\.\.?/|/)[A-Za-z0-9._~!$&()*+,;=:@%/-]+|--[A-Za-z0-9][A-Za-z0-9_-]*|\bv?\d+(?:\.\d+)*(?:[-+][A-Za-z0-9.-]+)?\b|\b[A-Za-z][A-Za-z0-9]*_[A-Za-z0-9_]+\b|\b[A-Za-z0-9]*[a-z][A-Za-z0-9]*[A-Z][A-Za-z0-9]*\b|\b[A-Z]{2,}[A-Za-z0-9]*\b`)

func preservesProtectedContent(doc *document.Document, start, end int, replacement string, terms []config.TermEntry) bool {
	if doc == nil || start < 0 || end < start || end > len(doc.Content) {
		return false
	}
	source := doc.Content[start:end]
	protected := map[string]struct{}{}
	for _, token := range protectedTokenPattern.FindAllString(source, -1) {
		token = strings.TrimRight(token, ".,;:!?)]}")
		if token != "" {
			protected[token] = struct{}{}
		}
	}
	for _, seg := range doc.Segments {
		if seg.Range.Start.Byte >= end || seg.Range.End.Byte <= start {
			continue
		}
		switch seg.Type {
		case document.SegmentInlineCode, document.SegmentLinkDest, document.SegmentInlineHTML:
			if seg.Text != "" {
				protected[seg.Text] = struct{}{}
			}
		}
	}
	for _, term := range terms {
		if term.Term != "" && strings.Contains(source, term.Term) {
			protected[term.Term] = struct{}{}
		}
	}
	for token := range protected {
		if !strings.Contains(replacement, token) {
			return false
		}
	}
	return true
}

// byteOffsetToLineColumn converts a zero-based byte offset into one-based
// line number and Unicode code-point column number (exclusive end).
// If byteOffset is at len(content), it returns the position of the
// (nonexistent) character after the last one.
func byteOffsetToLineColumn(content string, byteOffset int) (line, col int) {
	if byteOffset <= 0 {
		return 1, 1
	}
	if byteOffset > len(content) {
		byteOffset = len(content)
	}
	line = 1
	col = 1
	bytes := 0
	for bytes < byteOffset {
		r, size := utf8.DecodeRuneInString(content[bytes:])
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		bytes += size
	}
	return
}
