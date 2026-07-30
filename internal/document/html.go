package document

import (
	stdhtml "html"
	"io"
	"strings"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

// SourceSpan is a half-open byte range in the original input.
type SourceSpan struct {
	StartByte int `json:"start_byte"`
	EndByte   int `json:"end_byte"`
}

// ProjectionSegment maps virtual text to raw HTML. Protected segments cannot
// be rewritten; synthetic separators map to their introducing tag.
type ProjectionSegment struct {
	StartByte int
	EndByte   int
	Source    SourceSpan
	Protected bool
	// Atomic keeps decoded entities separate from adjacent literal source spans.
	Atomic bool
}

// HTMLProjection is the provenance-preserving visible-text view of HTML.
type HTMLProjection struct {
	Text     string
	Segments []ProjectionSegment
}

var hiddenHTMLTags = map[string]bool{
	"head": true, "script": true, "style": true, "template": true,
	"noscript": true,
}

var blockHTMLTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"div": true, "dl": true, "fieldset": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "header": true, "hr": true, "li": true,
	"main": true, "nav": true, "ol": true, "p": true, "pre": true,
	"section": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true, "ul": true,
}

var voidHTMLTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// ExtractHTMLVisibleText extracts visible text and maps it to unchanged raw HTML.
// It does not convert HTML to Markdown. Visibility excludes comments, doctypes,
// and head, script, style, template, and noscript. It does not evaluate CSS,
// JavaScript, or generated content.
func ExtractHTMLVisibleText(content string) HTMLProjection {
	var out strings.Builder
	var segments []ProjectionSegment
	var stack []string

	appendText := func(text string, source SourceSpan, protected bool) {
		if text == "" {
			return
		}
		start := out.Len()
		out.WriteString(text)
		end := out.Len()
		if len(segments) > 0 {
			last := &segments[len(segments)-1]
			if !last.Atomic && last.EndByte == start && last.Protected == protected && last.Source.EndByte == source.StartByte {
				last.EndByte = end
				last.Source.EndByte = source.EndByte
				return
			}
		}
		segments = append(segments, ProjectionSegment{StartByte: start, EndByte: end, Source: source, Protected: protected})
	}
	appendBoundary := func(source SourceSpan) {
		if out.Len() == 0 {
			return
		}
		current := out.String()
		if strings.HasSuffix(current, "\n") || strings.HasSuffix(current, " ") || strings.HasSuffix(current, "\t") {
			return
		}
		start := out.Len()
		out.WriteByte('\n')
		segments = append(segments, ProjectionSegment{StartByte: start, EndByte: start + 1, Source: source, Atomic: true})
	}
	inHidden := func() bool {
		for _, tag := range stack {
			if hiddenHTMLTags[tag] {
				return true
			}
		}
		return false
	}
	inCode := func() bool {
		for _, tag := range stack {
			if tag == "code" || tag == "pre" {
				return true
			}
		}
		return false
	}
	inLink := func() bool {
		for _, tag := range stack {
			if tag == "a" {
				return true
			}
		}
		return false
	}

	z := xhtml.NewTokenizer(strings.NewReader(content))
	rawOffset := 0
	for {
		tokenType := z.Next()
		raw := string(z.Raw())
		tokenStart := rawOffset
		rawOffset += len(raw)
		source := SourceSpan{StartByte: tokenStart, EndByte: rawOffset}

		switch tokenType {
		case xhtml.ErrorToken:
			if z.Err() != nil && z.Err() != io.EOF {
				// Keep the already extracted, mapped prefix after a tokenizer error.
			}
			return HTMLProjection{Text: out.String(), Segments: segments}

		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			nameBytes, _ := z.TagName()
			tag := strings.ToLower(string(nameBytes))
			if blockHTMLTags[tag] || tag == "br" {
				appendBoundary(source)
			}
			if tokenType != xhtml.SelfClosingTagToken && !voidHTMLTags[tag] {
				stack = append(stack, tag)
			}

		case xhtml.EndTagToken:
			nameBytes, _ := z.TagName()
			tag := strings.ToLower(string(nameBytes))
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i] == tag {
					stack = stack[:i]
					break
				}
			}
			if blockHTMLTags[tag] {
				appendBoundary(source)
			}

		case xhtml.TextToken:
			if inHidden() {
				continue
			}
			if inCode() {
				// Exclude code and preformatted text. The protected separator prevents
				// neighboring prose from becoming one token.
				appendText(" ", source, true)
				continue
			}
			// Keep link labels as context but prohibit their rewrite.
			appendDecodedHTMLText(&out, &segments, raw, tokenStart, inLink())
		}
	}
}

// appendDecodedHTMLText maps literal runs and decoded entities to raw source spans.
func appendDecodedHTMLText(out *strings.Builder, segments *[]ProjectionSegment, raw string, rawStart int, protected bool) {
	appendPiece := func(text string, source SourceSpan, atomic bool) {
		if text == "" {
			return
		}
		start := out.Len()
		out.WriteString(text)
		end := out.Len()
		if !atomic && len(*segments) > 0 {
			last := &(*segments)[len(*segments)-1]
			if !last.Atomic && last.EndByte == start && last.Protected == protected && last.Source.EndByte == source.StartByte {
				last.EndByte = end
				last.Source.EndByte = source.EndByte
				return
			}
		}
		*segments = append(*segments, ProjectionSegment{StartByte: start, EndByte: end, Source: source, Protected: protected, Atomic: atomic})
	}

	literalStart := 0
	for i := 0; i < len(raw); {
		if raw[i] == '&' {
			if semi := strings.IndexByte(raw[i:], ';'); semi >= 0 {
				end := i + semi + 1
				candidate := raw[i:end]
				decoded := stdhtml.UnescapeString(candidate)
				if decoded != candidate {
					appendPiece(raw[literalStart:i], SourceSpan{StartByte: rawStart + literalStart, EndByte: rawStart + i}, false)
					appendPiece(decoded, SourceSpan{StartByte: rawStart + i, EndByte: rawStart + end}, true)
					i = end
					literalStart = end
					continue
				}
			}
		}
		_, size := utf8.DecodeRuneInString(raw[i:])
		if size == 0 {
			break
		}
		i += size
	}
	appendPiece(raw[literalStart:], SourceSpan{StartByte: rawStart + literalStart, EndByte: rawStart + len(raw)}, false)
}

// AnalysisContent returns the text whose coordinate space is used for analysis.
func (d *Document) AnalysisContent() string {
	if d != nil && d.Format == FormatHTML {
		return d.Analysis
	}
	if d == nil {
		return ""
	}
	return d.Content
}

// SourceSpansForAnalysisRange returns raw HTML spans for a virtual range.
// Markdown and text use an identity range.
func (d *Document) SourceSpansForAnalysisRange(start, end int) []SourceSpan {
	if d == nil || start < 0 || end < start || end > len(d.AnalysisContent()) {
		return nil
	}
	if d.Format != FormatHTML {
		return []SourceSpan{{StartByte: start, EndByte: end}}
	}
	var spans []SourceSpan
	for _, segment := range d.Projection {
		if segment.StartByte >= end || segment.EndByte <= start {
			continue
		}
		span := segment.Source
		overlapStart := start
		if segment.StartByte > overlapStart {
			overlapStart = segment.StartByte
		}
		overlapEnd := end
		if segment.EndByte < overlapEnd {
			overlapEnd = segment.EndByte
		}
		// Literal text maps byte-for-byte to source. Entities and synthetic
		// separators retain full raw spans because their lengths differ.
		if !segment.Atomic && segment.Source.EndByte-segment.Source.StartByte == segment.EndByte-segment.StartByte {
			span.StartByte += overlapStart - segment.StartByte
			span.EndByte = span.StartByte + overlapEnd - overlapStart
		}
		spans = append(spans, span)
	}
	return spans
}

// IsProtectedAnalysisRange reports whether a rewrite touches protected text.
func (d *Document) IsProtectedAnalysisRange(start, end int) bool {
	if d == nil || d.Format != FormatHTML {
		return false
	}
	for _, segment := range d.Projection {
		if segment.Protected && segment.StartByte < end && start < segment.EndByte {
			return true
		}
	}
	return false
}
