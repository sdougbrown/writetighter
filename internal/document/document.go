package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type Position struct{ Line, Column, Byte int }
type Range struct{ Start, End Position }

type SegmentType string

const (
	SegmentProse       SegmentType = "prose"
	SegmentCodeBlock   SegmentType = "code_block"
	SegmentInlineCode  SegmentType = "inline_code"
	SegmentFrontMatter SegmentType = "front_matter"
	SegmentLinkDest    SegmentType = "link_destination"
	SegmentHTMLBlock   SegmentType = "html_block"
)

type Segment struct {
	Type  SegmentType
	Range Range
	Text  string
}

type Document struct {
	Source   string
	Kind     string
	Content  string
	Segments []Segment
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
	return &Document{Source: source, Kind: kind, Content: content, Segments: segmentMarkdown(content)}, nil
}

// maxAggregateBytes is the maximum total input size across all selected files.
const maxAggregateBytes = 25 * 1024 * 1024

// maxFileBytes is the maximum size for a single file.
const maxFileBytes = 5 * 1024 * 1024

func CollectInputs(paths []string, stdin bool) ([]*Document, error) {
	if stdin {
		// Limit stdin to maxAggregateBytes + 1 so we can detect overflow.
		limited := io.LimitReader(os.Stdin, int64(maxAggregateBytes+1))
		doc, err := FromReader(limited, "<stdin>", "")
		if err != nil {
			return nil, err
		}
		if len(doc.Content) > maxAggregateBytes {
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
		if info.Size() > maxFileBytes {
			return nil, fmt.Errorf("file too large: %s", path)
		}
		doc, err := FromFile(path, "")
		if err != nil {
			return nil, err
		}
		total += int64(len(doc.Content))
		if total > maxAggregateBytes {
			return nil, errors.New("aggregate input too large")
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// readFileWithBound reads a file but caps actual bytes read at maxFileBytes+1
// to detect growth or read-after-stat size changes.
func readFileWithBound(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	limited := io.LimitReader(f, int64(maxFileBytes+1))
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
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
	return &Document{Source: path, Kind: kind, Content: content, Segments: segmentMarkdown(content)}, nil
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
		if info.Size() > maxFileBytes {
			return nil, fmt.Errorf("file too large: %s", path)
		}
		doc, err := FromFile(path, "")
		if err != nil {
			return nil, err
		}
		*total += int64(len(doc.Content))
		if *total > maxAggregateBytes {
			return nil, errors.New("aggregate input too large")
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func isAllowed(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".txt":
		return true
	default:
		return false
	}
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
			end := findBlockEnd(content, lineStart)
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
			j := i + 1
			for j < len(line) && line[j] != '`' {
				j++
			}
			if j < len(line) {
				if i > last {
					segs = append(segs, makeSeg(SegmentProse, line[last:i], start+last, start+i, content))
				}
				segs = append(segs, makeSeg(SegmentInlineCode, line[i:j+1], start+i, start+j+1, content))
				last = j + 1
				i = j + 1
				continue
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
func isHTMLBlock(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "<") && strings.HasSuffix(strings.TrimSpace(line), ">")
}
func findBlockEnd(content string, start int) int {
	if idx := strings.Index(content[start:], "\n\n"); idx >= 0 {
		return start + idx
	}
	return len(content)
}
