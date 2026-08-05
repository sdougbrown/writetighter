// Package codecomment extracts immutable, language-aware comment catalogs.
//
// It intentionally recognizes only a bounded set of languages and fails closed
// on unterminated literals or comments. Callers must not use a partial catalog
// to locate editable source.
package codecomment

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Language identifies a source language supported by this experimental catalog.
type Language string

const (
	Go         Language = "go"
	TypeScript Language = "ts"
	Rust       Language = "rust"
	Python     Language = "py"
)

// CommentForm identifies the lexical delimiter family of a comment unit.
type CommentForm string

const (
	LineComment  CommentForm = "line"
	BlockComment CommentForm = "block"
)

// Span is a half-open UTF-8 byte range and its one-based source line range.
type Span struct {
	StartByte int `json:"start_byte"`
	EndByte   int `json:"end_byte"`
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

// Comment is a stable, file-local editable comment unit.
type Comment struct {
	ID   string      `json:"id"`
	Span Span        `json:"span"`
	Text string      `json:"text"`
	Form CommentForm `json:"form"`
}

// Catalog maps every editable comment unit in one immutable source file.
type Catalog struct {
	File         string    `json:"file"`
	Language     Language  `json:"language"`
	SourceSHA256 string    `json:"source_sha256"`
	Comments     []Comment `json:"comments"`
}

// DetectLanguage detects a supported language from filename's extension.
func DetectLanguage(filename string) (Language, bool) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".go":
		return Go, true
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return TypeScript, true
	case ".rs":
		return Rust, true
	case ".py", ".pyi":
		return Python, true
	default:
		return "", false
	}
}

// ParseLanguage validates an explicit language argument.
func ParseLanguage(value string) (Language, error) {
	switch strings.ToLower(value) {
	case "go":
		return Go, nil
	case "ts", "typescript", "js", "javascript":
		return TypeScript, nil
	case "rust", "rs":
		return Rust, nil
	case "py", "python":
		return Python, nil
	default:
		return "", fmt.Errorf("unsupported comment language %q", value)
	}
}

// Extract produces a catalog for source. filename is metadata only, except
// callers may use DetectLanguage before calling it. source must be valid UTF-8.
func Extract(filename string, language Language, source []byte) (Catalog, error) {
	if !utf8.Valid(source) {
		return Catalog{}, fmt.Errorf("%s: source is not valid UTF-8", filename)
	}
	var (
		tokens []commentToken
		err    error
	)
	switch language {
	case Go:
		tokens, err = scanGo(source)
	case TypeScript:
		tokens, err = scanTypeScript(source)
	case Rust:
		tokens, err = scanRust(source)
	case Python:
		tokens, err = scanPython(source)
	default:
		return Catalog{}, fmt.Errorf("%s: unsupported comment language %q", filename, language)
	}
	if err != nil {
		return Catalog{}, fmt.Errorf("%s: comment extraction: %w", filename, err)
	}

	hash := sha256.Sum256(source)
	units := coalesce(source, tokens)
	comments := make([]Comment, len(units))
	for i, unit := range units {
		comments[i] = Comment{
			ID: fmt.Sprintf("c%04d", i+1),
			Span: Span{
				StartByte: unit.start,
				EndByte:   unit.end,
				StartLine: lineAt(source, unit.start),
				EndLine:   lineAt(source, lastByte(unit.start, unit.end)),
			},
			Text: string(source[unit.start:unit.end]),
			Form: unit.form,
		}
	}
	return Catalog{File: filename, Language: language, SourceSHA256: fmt.Sprintf("%x", hash[:]), Comments: comments}, nil
}

type commentToken struct {
	start, end int
	form       CommentForm
}

func lastByte(start, end int) int {
	if end > start {
		return end - 1
	}
	return start
}

func lineAt(source []byte, offset int) int {
	line := 1
	for i := 0; i < offset; i++ {
		if source[i] == '\n' || (source[i] == '\r' && (i+1 == len(source) || source[i+1] != '\n')) {
			line++
		}
	}
	return line
}

func lineEnd(source []byte, start int) int {
	for i := start; i < len(source); i++ {
		if source[i] == '\r' || source[i] == '\n' {
			return i
		}
	}
	return len(source)
}

func isLineBreakAt(source []byte, i int) bool {
	return i < len(source) && (source[i] == '\r' || source[i] == '\n')
}

func skipLineBreak(source []byte, i int) int {
	if i < len(source) && source[i] == '\r' {
		i++
		if i < len(source) && source[i] == '\n' {
			i++
		}
		return i
	}
	if i < len(source) && source[i] == '\n' {
		return i + 1
	}
	return i
}

func coalesce(source []byte, tokens []commentToken) []commentToken {
	if len(tokens) == 0 {
		return nil
	}
	units := make([]commentToken, 0, len(tokens))
	for _, token := range tokens {
		if len(units) > 0 {
			previous := &units[len(units)-1]
			if previous.form == token.form && isFullLineComment(source, previous.start, previous.end) &&
				isFullLineComment(source, token.start, token.end) && sameIndent(source, previous.start, token.start) &&
				noBlankLine(source, previous.end, token.start) {
				previous.end = token.end
				continue
			}
		}
		units = append(units, token)
	}
	return units
}

func isFullLineComment(source []byte, start, end int) bool {
	lineStart := start
	for lineStart > 0 && source[lineStart-1] != '\n' && source[lineStart-1] != '\r' {
		lineStart--
	}
	for i := lineStart; i < start; i++ {
		if source[i] != ' ' && source[i] != '\t' {
			return false
		}
	}
	for i := end; i < len(source) && source[i] != '\n' && source[i] != '\r'; i++ {
		if source[i] != ' ' && source[i] != '\t' {
			return false
		}
	}
	return true
}

func indentation(source []byte, start int) []byte {
	lineStart := start
	for lineStart > 0 && source[lineStart-1] != '\n' && source[lineStart-1] != '\r' {
		lineStart--
	}
	return source[lineStart:start]
}

func sameIndent(source []byte, first, second int) bool {
	return string(indentation(source, first)) == string(indentation(source, second))
}

func noBlankLine(source []byte, end, next int) bool {
	// The range between separate comment tokens must contain exactly one line
	// break and indentation. Anything else, including a second line break, is a
	// blank line or code and prevents coalescing.
	i := end
	if i >= next || !isLineBreakAt(source, i) {
		return false
	}
	i = skipLineBreak(source, i)
	for i < next {
		if source[i] != ' ' && source[i] != '\t' {
			return false
		}
		i++
	}
	return true
}
