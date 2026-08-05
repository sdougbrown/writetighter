package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/limits"
)

type Position struct{ Line, Column, Byte int }
type Range struct{ Start, End Position }

type SegmentType string

const (
	SegmentProse       SegmentType = "prose"
	SegmentCodeBlock   SegmentType = "code_block"
	SegmentInlineCode  SegmentType = "inline_code"
	SegmentInlineHTML  SegmentType = "inline_html"
	SegmentFrontMatter SegmentType = "front_matter"
	SegmentLinkDest    SegmentType = "link_destination"
	SegmentHTMLBlock   SegmentType = "html_block"
)

type Segment struct {
	Type  SegmentType
	Range Range
	Text  string
}

type DocumentFormat string

const (
	FormatMarkdown DocumentFormat = "markdown"
	FormatText     DocumentFormat = "text"
	FormatHTML     DocumentFormat = "html"
)

type Document struct {
	Source     string
	Kind       string
	Format     DocumentFormat
	Content    string // exact original input; never transformed
	Analysis   string // virtual visible text for HTML; empty otherwise
	Projection []ProjectionSegment
	Segments   []Segment
}

// ChunkRange is a half-open byte range in a document.
type ChunkRange struct {
	StartByte int
	EndByte   int
}

func FromReader(r io.Reader, source, kind string) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errors.New("invalid UTF-8")
	}
	content := string(data)
	return newDocument(source, kind, content), nil
}

// FromText creates a bounded virtual document from a command-line text value.
func FromText(text, kind string) (*Document, error) {
	if len(text) > limits.MaxAggregateBytes {
		return nil, errors.New("text input too large")
	}
	return FromReader(strings.NewReader(text), "<text>", kind)
}

func CollectInputs(paths []string, stdin bool) ([]*Document, error) {
	if stdin {
		// Limit stdin to limits.MaxAggregateBytes + 1 so we can detect overflow.
		limited := io.LimitReader(os.Stdin, int64(limits.MaxAggregateBytes+1))
		doc, err := FromReader(limited, "<stdin>", "")
		if err != nil {
			return nil, err
		}
		if len(doc.Content) > limits.MaxAggregateBytes {
			return nil, errors.New("stdin input too large")
		}
		return []*Document{doc}, nil
	}
	var docs []*Document
	var total int64
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("cannot follow symlink: %s", path)
		}
		if info.IsDir() {
			more, err := collectDir(path, &total)
			if err != nil {
				return nil, err
			}
			docs = append(docs, more...)
			continue
		}
		if info.Size() > limits.MaxFileBytes {
			return nil, fmt.Errorf("file too large: %s", path)
		}
		doc, err := FromFile(path, "")
		if err != nil {
			return nil, err
		}
		total += int64(len(doc.Content))
		if total > limits.MaxAggregateBytes {
			return nil, errors.New("aggregate input too large")
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// readFileWithBound reads a file but caps actual bytes read at limits.MaxFileBytes+1
// to detect growth or read-after-stat size changes.
func readFileWithBound(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	limited := io.LimitReader(f, int64(limits.MaxFileBytes+1))
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > limits.MaxFileBytes {
		return nil, fmt.Errorf("file too large: %s", path)
	}
	return data, nil
}

func FromFile(path, kind string) (*Document, error) {
	data, err := readFileWithBound(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errors.New("invalid UTF-8")
	}
	content := string(data)
	return newDocument(path, kind, content), nil
}

func collectDir(root string, total *int64) ([]*Document, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !isAllowed(path) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var docs []*Document
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.Size() > limits.MaxFileBytes {
			return nil, fmt.Errorf("file too large: %s", path)
		}
		doc, err := FromFile(path, "")
		if err != nil {
			return nil, err
		}
		*total += int64(len(doc.Content))
		if *total > limits.MaxAggregateBytes {
			return nil, errors.New("aggregate input too large")
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// ChunkRanges splits a document at Markdown block boundaries when possible.
// It preserves complete, non-overlapping coverage and UTF-8 boundaries.
func ChunkRanges(doc *Document, maxBytes int) []ChunkRange {
	if doc == nil || len(doc.AnalysisContent()) == 0 || maxBytes <= 0 {
		return nil
	}
	content := doc.AnalysisContent()
	atomic := make([]Range, 0)
	for _, seg := range doc.Segments {
		switch seg.Type {
		case SegmentCodeBlock, SegmentFrontMatter, SegmentHTMLBlock:
			atomic = append(atomic, seg.Range)
		}
	}
	chunks := make([]ChunkRange, 0, (len(content)+maxBytes-1)/maxBytes)
	for start := 0; start < len(content); {
		limit := start + maxBytes
		if limit >= len(content) {
			chunks = append(chunks, ChunkRange{StartByte: start, EndByte: len(content)})
			break
		}
		for limit > start && !utf8.RuneStart(content[limit]) {
			limit--
		}
		end := preferredChunkEnd(content, start, limit)
		for _, block := range atomic {
			if block.Start.Byte < end && end < block.End.Byte {
				if block.Start.Byte > start {
					end = block.Start.Byte
				} else if block.End.Byte-start <= maxBytes {
					end = block.End.Byte
				} else {
					end = limit
				}
				break
			}
		}
		if end <= start {
			end = limit
		}
		chunks = append(chunks, ChunkRange{StartByte: start, EndByte: end})
		start = end
	}
	return chunks
}

func preferredChunkEnd(content string, start, limit int) int {
	window := content[start:limit]
	if split := strings.LastIndex(window, "\n\n"); split >= 0 {
		return start + split + 2
	}
	if split := strings.LastIndexByte(window, '\n'); split >= 0 {
		return start + split + 1
	}
	return limit
}

func isAllowed(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".txt", ".html", ".htm":
		return true
	default:
		return false
	}
}

func newDocument(source, kind, content string) *Document {
	format := detectFormat(source, content)
	doc := &Document{Source: source, Kind: kind, Format: format, Content: content}
	if format == FormatHTML {
		projection := ExtractHTMLVisibleText(content)
		doc.Analysis = projection.Text
		doc.Projection = projection.Segments
		return doc
	}
	doc.Segments = segmentMarkdown(content)
	return doc
}

func detectFormat(source, content string) DocumentFormat {
	switch strings.ToLower(filepath.Ext(source)) {
	case ".html", ".htm":
		return FormatHTML
	case ".txt":
		return FormatText
	case ".md", ".markdown":
		return FormatMarkdown
	}
	// stdin and --text can provide an HTML fragment.
	prefix := content
	if len(prefix) > 1024 {
		prefix = prefix[:1024]
	}
	lower := strings.ToLower(prefix)
	if strings.Contains(lower, "<!doctype") || strings.Contains(lower, "<html") {
		return FormatHTML
	}
	return FormatMarkdown
}

func segmentMarkdown(content string) []Segment {
	if content == "" {
		return nil
	}
	var segs []Segment
	lineStart := 0
	for lineStart <= len(content) {
		idx := strings.IndexByte(content[lineStart:], '\n')
		lineEnd := len(content)
		if idx >= 0 {
			lineEnd = lineStart + idx
		}
		if lineEnd < lineStart {
			lineEnd = len(content)
		}
		line := content[lineStart:lineEnd]
		if lineStart == 0 && isFrontMatterStart(line) {
			end := findFrontMatterEnd(content)
			if end > 0 {
				segs = append(segs, makeSeg(SegmentFrontMatter, content[:end], 0, end, content))
				lineStart = end
				continue
			}
		}
		if isFence(line) {
			end := findFenceEnd(content, lineStart, line)
			if end > lineStart {
				segs = append(segs, makeSeg(SegmentCodeBlock, content[lineStart:end], lineStart, end, content))
				lineStart = end
				continue
			}
		}
		if isHTMLBlock(line) {
			end := findHTMLBlockEnd(content, lineStart, lineEnd, line)
			segs = append(segs, makeSeg(SegmentHTMLBlock, content[lineStart:end], lineStart, end, content))
			lineStart = end
			continue
		}
		segs = append(segs, segmentProseLine(content, lineStart, lineEnd)...)
		if lineEnd == len(content) {
			break
		}
		lineStart = lineEnd + 1
	}
	return mergeAdjacent(segs)
}

func segmentProseLine(content string, start, end int) []Segment {
	line := content[start:end]
	if line == "" {
		return nil
	}
	var segs []Segment
	last := 0
	for i := 0; i < len(line); {
		if line[i] == '`' {
			// Count opening backticks
			n := 0
			for i+n < len(line) && line[i+n] == '`' {
				n++
			}
			// Search for closing run of exactly n backticks
			j := i + n
			found := false
			for j < len(line) {
				if line[j] == '`' {
					k := 0
					for j+k < len(line) && line[j+k] == '`' {
						k++
					}
					if k == n {
						if i > last {
							segs = append(segs, makeSeg(SegmentProse, line[last:i], start+last, start+i, content))
						}
						segs = append(segs, makeSeg(SegmentInlineCode, line[i:j+k], start+i, start+j+k, content))
						last = j + k
						i = j + k
						found = true
						break
					}
					j += k
				} else {
					j++
				}
			}
			if found {
				continue
			}
		}
		if line[i] == '<' {
			if j := strings.IndexByte(line[i+1:], '>'); j >= 0 {
				j += i + 1
				inner := line[i+1 : j]
				segmentType := SegmentType("")
				if isAutolinkDestination(inner) {
					segmentType = SegmentLinkDest
				} else if isInlineHTMLTag(inner) {
					segmentType = SegmentInlineHTML
				}
				if segmentType != "" {
					if i > last {
						segs = append(segs, makeSeg(SegmentProse, line[last:i], start+last, start+i, content))
					}
					segs = append(segs, makeSeg(segmentType, line[i:j+1], start+i, start+j+1, content))
					last = j + 1
					i = j + 1
					continue
				}
			}
		}
		if line[i] == '(' && i > 0 && line[i-1] == ']' {
			j := i + 1
			depth := 1
			for j < len(line) && depth > 0 {
				if line[j] == '(' {
					depth++
				} else if line[j] == ')' {
					depth--
				}
				if depth > 0 {
					j++
				}
			}
			if j < len(line) && line[j] == ')' {
				if i > last {
					segs = append(segs, makeSeg(SegmentProse, line[last:i], start+last, start+i, content))
				}
				segs = append(segs, makeSeg(SegmentLinkDest, line[i:j+1], start+i, start+j+1, content))
				last = j + 1
				i = j
				continue
			}
		}
		i++
	}
	if last < len(line) {
		segs = append(segs, makeSeg(SegmentProse, line[last:], start+last, end, content))
	}
	return segs
}

func makeSeg(t SegmentType, text string, start, end int, content string) Segment {
	return Segment{Type: t, Range: positionsFromOffsets(content, start, end), Text: text}
}

func positionsFromOffsets(content string, startByte, endByte int) Range {
	return Range{
		Start: offsetToPos(content, startByte),
		End:   offsetToPos(content, endByte),
	}
}

func offsetToPos(content string, byteOffset int) Position {
	if byteOffset <= 0 {
		return Position{Line: 1, Column: 1, Byte: 0}
	}
	if byteOffset > len(content) {
		byteOffset = len(content)
	}
	line, col := 1, 1
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
	return Position{Line: line, Column: col, Byte: byteOffset}
}

func mergeAdjacent(segs []Segment) []Segment { return segs }

func isFrontMatterStart(line string) bool {
	return strings.TrimSpace(line) == "---" || strings.TrimSpace(line) == "+++"
}
func findFrontMatterEnd(content string) int {
	start := strings.IndexByte(content, '\n')
	if start < 0 {
		return 0
	}

	rest := content[start+1:]
	restLen := len(rest)
	i := 0
	for i < restLen {
		remainder := rest[i:]
		if strings.HasPrefix(remainder, "---") || strings.HasPrefix(remainder, "+++") {
			endOfFence := i + 3
			if endOfFence >= restLen || rest[endOfFence] == '\n' || rest[endOfFence] == '\r' {
				closeEnd := endOfFence
				if closeEnd < restLen && rest[closeEnd] == '\r' {
					closeEnd++
				}
				if closeEnd < restLen && rest[closeEnd] == '\n' {
					closeEnd++
				}
				return start + 1 + closeEnd
			}
		}

		nextNewline := strings.IndexByte(rest[i:], '\n')
		if nextNewline < 0 {
			break
		}
		i += nextNewline + 1
	}

	return 0
}
func isFence(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "```") }
func findFenceEnd(content string, start int, line string) int {
	if idx := strings.Index(content[start+len(line):], "\n```"); idx >= 0 {
		return start + len(line) + idx + 4
	}
	return len(content)
}

var htmlBlockTags = []string{
	"<address", "<article", "<aside", "<base", "<basefont", "<blockquote",
	"<body", "<caption", "<center", "<col", "<colgroup", "<dd", "<details",
	"<dialog", "<dir", "<div", "<dl", "<dt", "<fieldset", "<figcaption",
	"<figure", "<footer", "<form", "<frame", "<frameset", "<h1", "<h2",
	"<h3", "<h4", "<h5", "<h6", "<head", "<header", "<hr", "<html",
	"<iframe", "<legend", "<li", "<link", "<main", "<menu", "<menuitem",
	"<nav", "<noframes", "<ol", "<optgroup", "<option", "<p", "<param",
	"<pre", "<script", "<search", "<section", "<style", "<summary", "<table",
	"<tbody", "<td", "<tfoot", "<th", "<thead", "<title", "<tr", "<track",
	"<ul",
}

func isHTMLBlock(line string) bool {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "<!--") || strings.HasPrefix(lower, "<![cdata[") ||
		strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<?") {
		return true
	}
	// Only recognize complete block-level tag names, not inline HTML such as
	// <em> or longer custom tags that merely share a prefix.
	tagText := lower
	if strings.HasPrefix(tagText, "</") {
		tagText = "<" + tagText[2:]
	}
	for _, tag := range htmlBlockTags {
		if !strings.HasPrefix(tagText, tag) {
			continue
		}
		if len(tagText) == len(tag) || strings.ContainsRune(" \t>/", rune(tagText[len(tag)])) {
			return true
		}
	}
	return false
}
func findHTMLBlockEnd(content string, start, lineEnd int, line string) int {
	lowerLine := strings.ToLower(strings.TrimSpace(line))
	rest := content[start:]
	lowerRest := strings.ToLower(rest)
	for _, delimiter := range []struct{ prefix, suffix string }{
		{"<!--", "-->"}, {"<?", "?>"}, {"<![cdata[", "]]>"},
	} {
		if strings.HasPrefix(lowerLine, delimiter.prefix) {
			if idx := strings.Index(lowerRest, delimiter.suffix); idx >= 0 {
				return start + idx + len(delimiter.suffix)
			}
			return len(content)
		}
	}
	for _, tag := range []string{"script", "style", "pre"} {
		if strings.HasPrefix(lowerLine, "<"+tag) {
			closing := "</" + tag + ">"
			if idx := strings.Index(lowerRest, closing); idx >= 0 {
				return start + idx + len(closing)
			}
			return len(content)
		}
	}
	// End any explicitly closed block at its closing tag rather than consuming
	// unrelated prose up to the next blank line.
	for _, tagPrefix := range htmlBlockTags {
		if !strings.HasPrefix(lowerLine, tagPrefix) {
			continue
		}
		name := tagPrefix[1:]
		closing := "</" + name + ">"
		if idx := strings.Index(lowerRest, closing); idx >= 0 {
			return start + idx + len(closing)
		}
		if strings.Contains(lowerLine, "/>") {
			return lineEnd
		}
		break
	}
	if strings.HasPrefix(lowerLine, "</") {
		return lineEnd
	}
	if idx := strings.Index(rest, "\n\n"); idx >= 0 {
		return start + idx
	}
	return len(content)
}

func isInlineHTMLTag(inner string) bool {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return false
	}
	if inner[0] == '/' {
		inner = inner[1:]
	}
	return inner != "" && unicode.IsLetter(rune(inner[0]))
}

func isAutolinkDestination(inner string) bool {
	if inner == "" || strings.IndexFunc(inner, unicode.IsSpace) >= 0 {
		return false
	}
	if at := strings.IndexByte(inner, '@'); at > 0 && at < len(inner)-1 {
		return true
	}
	colon := strings.IndexByte(inner, ':')
	if colon < 2 || colon > 32 || !unicode.IsLetter(rune(inner[0])) {
		return false
	}
	for _, r := range inner[1:colon] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '.' && r != '-' {
			return false
		}
	}
	return true
}
