package reference

import (
	"bytes"
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

// Entry represents one collected reference file.
type Entry struct {
	DisplayPath   string // Human-readable path (lexical for directories)
	SourcePath    string // Original filesystem path
	Content       string // UTF-8 validated content
	InputBytes    int    // Original file size
	IncludedBytes int    // Size of content after processing (HTML projection etc.)
}

// Pack holds a bounded, deterministic set of reference entries.
type Pack struct {
	Entries       []Entry
	InputBytes    int  // Total input bytes across all entries
	IncludedBytes int  // Total included bytes after processing (rendered size)
	Complete      bool // True if all paths were fully included
}

// ErrNoFit is returned when reference pack cannot fit in the available budget.
type ErrNoFit struct {
	Available int
	Required  int
}

func (e *ErrNoFit) Error() string {
	return fmt.Sprintf("reference pack requires %d bytes but only %d available", e.Required, e.Available)
}

// Collect reads the given paths and returns a Pack.
// Files: read directly if bounded valid UTF-8 (any extension)
// Directories: recursive with allowlist filtering.
// References whose canonical path matches any sourcePath are excluded.
func Collect(paths []string, sourcePaths []string) (*Pack, error) {
	// Canonicalize source paths for exclusion matching.
	sourceSet := make(map[string]bool)
	for _, sp := range sourcePaths {
		canon, err := canonicalPath(sp)
		if err != nil {
			return nil, fmt.Errorf("source path %q: %w", sp, err)
		}
		sourceSet[canon] = true
	}

	var entries []Entry
	seenCanon := make(map[string]bool) // canonical path dedup
	totalInput := 0

	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("accessing %q: %w", path, err)
		}

		// Reject symlinks at top level.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("cannot follow symlink: %s", path)
		}

		if info.IsDir() {
			dirEntries, err := collectDirectory(path, sourceSet, seenCanon, &totalInput)
			if err != nil {
				return nil, err
			}
			entries = append(entries, dirEntries...)
			continue
		}

		// Explicit file path: any extension allowed, subject to limits and validation.
		entry, err := collectFile(path, path, sourceSet, seenCanon, &totalInput)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].DisplayPath < entries[j].DisplayPath
	})

	pack := &Pack{
		Entries:    entries,
		InputBytes: totalInput,
		Complete:   true,
	}

	// Compute total rendered size.
	rendered := pack.Render()
	pack.IncludedBytes = len(rendered)

	return pack, nil
}

// collectDirectory walks a directory recursively and collects matching reference files.
func collectDirectory(root string, sourceSet map[string]bool, seenCanon map[string]bool, totalInput *int) ([]Entry, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := d.Name()

		// Skip hidden directories and .git.
		if d.IsDir() {
			if path != root && (strings.HasPrefix(base, ".") || base == ".git") {
				return filepath.SkipDir
			}
			return nil
		}

		// Reject symlinks at any depth.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		// Check secret/binary filename patterns — silently skip.
		if isSecretFile(base) {
			return nil
		}

		// Only include files with allowed extensions.
		if !isAllowedExtension(base) {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)

	var entries []Entry
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		displayPath := filepath.Join(filepath.Base(root), rel)

		entry, err := collectFile(path, displayPath, sourceSet, seenCanon, totalInput)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
	}
	return entries, nil
}

// collectFile reads, validates, and processes a single reference file.
// Returns nil entry if the file is skipped due to source path exclusion.
func collectFile(path, displayPath string, sourceSet map[string]bool, seenCanon map[string]bool, totalInput *int) (*Entry, error) {
	// Resolve canonical path for dedup and source exclusion.
	canon, err := canonicalPath(path)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", path, err)
	}

	// Skip if already seen (dedup).
	if seenCanon[canon] {
		return nil, nil
	}

	// Skip if matches a source path.
	if sourceSet[canon] {
		return nil, nil
	}

	// Check secret/binary filename patterns — explicit file match returns error.
	if isSecretFile(filepath.Base(path)) {
		return nil, fmt.Errorf("reference path %q matches a secret/binary filename pattern", path)
	}

	// Check file size via stat first.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if info.Size() > limits.MaxFileBytes {
		return nil, fmt.Errorf("reference file too large: %s (%d bytes)", path, info.Size())
	}

	// Read file content with size bound.
	data, err := readFileBounded(path)
	if err != nil {
		return nil, err
	}

	// Reject binary files (null bytes).
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("reference file %q contains binary data (null bytes)", path)
	}

	// Validate UTF-8.
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("reference file %q contains invalid UTF-8", path)
	}

	content := string(data)

	// Process content based on file extension.
	ext := strings.ToLower(filepath.Ext(path))
	var processed string
	switch ext {
	case ".html", ".htm":
		processed = htmlToVisibleText(content)
	default:
		processed = content
	}

	entry := Entry{
		DisplayPath:   displayPath,
		SourcePath:    path,
		Content:       processed,
		InputBytes:    len(data),
		IncludedBytes: len(processed),
	}

	// Update aggregate tracking.
	*totalInput += len(data)
	if *totalInput > limits.MaxAggregateBytes {
		return nil, errors.New("aggregate reference input too large")
	}

	seenCanon[canon] = true

	return &entry, nil
}

// Render produces the wrapped reference text for inclusion in a prompt.
// Each entry is rendered as:
//
//	<reference file="display/path.md">
//	content here
//	</reference>
func (p *Pack) Render() string {
	var b strings.Builder
	for i, entry := range p.Entries {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "<reference file=%q>\n", entry.DisplayPath)
		b.WriteString(entry.Content)
		if !strings.HasSuffix(entry.Content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("</reference>\n")
	}
	return b.String()
}

// canonicalPath resolves a path to its canonical form (absolute, symlink-evaluated).
func canonicalPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	evaled, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// If path doesn't exist, still return cleaned as absolute.
		if os.IsNotExist(err) {
			abs, err := filepath.Abs(cleaned)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
		return "", err
	}
	// Ensure result is absolute for consistent dedup.
	abs, err := filepath.Abs(evaled)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// readFileBounded reads a file with a limit of MaxFileBytes+1 to detect oversized files.
func readFileBounded(path string) ([]byte, error) {
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

// isSecretFile checks if a base filename matches secret/binary patterns.
func isSecretFile(name string) bool {
	// .env files with any suffix.
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, ".env") {
		return true
	}

	// Restricted extensions.
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".pem", ".key", ".cert", ".crt", ".cer", ".der",
		".p12", ".pfx", ".jks", ".keystore", ".kdb", ".ps1":
		return true
	}

	// SSH private-key filenames have no extension.
	switch lower {
	case "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "id_xmss":
		return true
	}
	return false
}

// isAllowedExtension checks if a file extension is in the reference allowlist.
func isAllowedExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md", ".markdown", ".txt", ".html", ".htm",
		".go", ".rs", ".py", ".js", ".jsx", ".ts", ".tsx",
		".java", ".c", ".h", ".cc", ".cpp", ".cxx", ".hh", ".hpp",
		".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf",
		".xml", ".csv",
		".sh", ".bash", ".zsh",
		".sql", ".proto":
		return true
	}
	return false
}

// htmlToVisibleText strips HTML tags from content and decodes common entities.
// This is a simplified version compared to the full provenance-preserving
// projection in internal/document; it is suitable for read-only reference context.
func htmlToVisibleText(content string) string {
	var b strings.Builder

	i := 0
	// Track whether we're inside a tag to suppress tag content.
	// Track whether we're inside a hidden element (script, style, etc.).
	inHidden := false
	var tagStack []string

	isHiddenTag := func(tag string) bool {
		switch tag {
		case "head", "script", "style", "template", "noscript":
			return true
		}
		return false
	}

	isBlockTag := func(tag string) bool {
		switch tag {
		case "address", "article", "aside", "blockquote", "div", "dl",
			"fieldset", "figure", "footer", "form", "h1", "h2", "h3",
			"h4", "h5", "h6", "header", "hr", "li", "main", "nav",
			"ol", "p", "pre", "section", "table", "tbody", "td", "tfoot",
			"th", "thead", "tr", "ul":
			return true
		}
		return false
	}

	lastWasSpace := true
	addSpace := func() {
		if !lastWasSpace {
			b.WriteByte(' ')
			lastWasSpace = true
		}
	}
	addNewline := func() {
		// Only add newline if we haven't just added one.
		s := b.String()
		if !strings.HasSuffix(s, "\n") {
			b.WriteByte('\n')
			lastWasSpace = true
		}
	}

	for i < len(content) {
		if content[i] == '<' {
			// Find end of tag.
			j := strings.IndexByte(content[i:], '>')
			if j < 0 {
				// Unclosed tag — skip to end.
				break
			}
			tagContent := content[i+1 : i+j]
			i += j + 1

			// Skip comments and doctypes.
			if strings.HasPrefix(tagContent, "!--") {
				if closeIdx := strings.Index(tagContent, "--"); closeIdx >= 0 {
					continue
				}
				continue
			}
			if strings.HasPrefix(tagContent, "![CDATA[") {
				if closeIdx := strings.Index(tagContent, "]]"); closeIdx >= 0 {
					continue
				}
				continue
			}
			if strings.HasPrefix(tagContent, "?") {
				continue
			}

			// Parse tag name.
			tagName := parseTagName(tagContent)
			isClosing := len(tagContent) > 0 && tagContent[0] == '/'

			if isClosing {
				// Pop from stack.
				for len(tagStack) > 0 && tagStack[len(tagStack)-1] != tagName {
					tagStack = tagStack[:len(tagStack)-1]
				}
				if len(tagStack) > 0 {
					tagStack = tagStack[:len(tagStack)-1]
				}
				inHidden = false
				for _, t := range tagStack {
					if isHiddenTag(t) {
						inHidden = true
						break
					}
				}
				if isBlockTag(tagName) {
					addNewline()
				}
			} else {
				// Check for self-closing: ends with / or void element.
				isSelfClosing := strings.HasSuffix(strings.TrimRight(tagContent, " \t"), "/") || isVoidElement(tagName)
				if !isSelfClosing && tagName != "" {
					tagStack = append(tagStack, tagName)
					if isHiddenTag(tagName) {
						inHidden = true
					}
				}
				if isBlockTag(tagName) || tagName == "br" {
					addNewline()
				}
			}
			continue
		}

		if inHidden {
			i++
			continue
		}

		// Decode entities and accumulate text.
		if content[i] == '&' {
			semi := strings.IndexByte(content[i:], ';')
			if semi >= 0 && semi <= 10 {
				entity := content[i : i+semi+1]
				decoded := decodeHTMLEntity(entity)
				if decoded != "" {
					// Check if decoded is a space/newline vs word char.
					for _, r := range decoded {
						if isWhitespaceRune(r) {
							addSpace()
						} else {
							b.WriteRune(r)
							lastWasSpace = false
						}
					}
				} else {
					// Unknown entity — emit raw.
					b.WriteString(entity)
					lastWasSpace = false
				}
				i += semi + 1
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(content[i:])
		if size == 0 {
			break
		}
		if isWhitespaceRune(r) {
			addSpace()
		} else {
			b.WriteRune(r)
			lastWasSpace = false
		}
		i += size
	}

	result := b.String()
	// Trim leading/trailing whitespace.
	result = strings.TrimSpace(result)
	return result
}

// parseTagName extracts the tag name from an HTML tag string (without angle brackets).
func parseTagName(tag string) string {
	tag = strings.TrimLeft(tag, "/")
	for i, r := range tag {
		if unicode.IsSpace(r) || r == '>' || r == '/' {
			return strings.ToLower(tag[:i])
		}
	}
	return strings.ToLower(tag)
}

// isVoidElement returns true for HTML void (self-closing) elements.
func isVoidElement(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// decodeHTMLEntity decodes common HTML entities.
func decodeHTMLEntity(entity string) string {
	switch entity {
	case "&amp;":
		return "&"
	case "&lt;":
		return "<"
	case "&gt;":
		return ">"
	case "&quot;":
		return "\""
	case "&apos;":
		return "'"
	case "&nbsp;":
		return " "
	case "&ndash;":
		return "–"
	case "&mdash;":
		return "—"
	case "&lsquo;":
		return "‘"
	case "&rsquo;":
		return "’"
	case "&ldquo;":
		return "“"
	case "&rdquo;":
		return "”"
	case "&hellip;":
		return "…"
	case "&bull;":
		return "•"
	case "&copy;":
		return "©"
	case "&reg;":
		return "®"
	case "&trade;":
		return "™"
	case "&euro;":
		return "€"
	case "&pound;":
		return "£"
	case "&yen;":
		return "¥"
	default:
		// Numeric entities.
		if strings.HasPrefix(entity, "&#x") || strings.HasPrefix(entity, "&#X") {
			var codepoint rune
			hexStr := entity[3 : len(entity)-1]
			if _, err := fmt.Sscanf(hexStr, "%x", &codepoint); err == nil {
				return string(codepoint)
			}
		} else if strings.HasPrefix(entity, "&#") {
			var codepoint rune
			decStr := entity[2 : len(entity)-1]
			if _, err := fmt.Sscanf(decStr, "%d", &codepoint); err == nil {
				return string(codepoint)
			}
		}
		return ""
	}
}

// isWhitespaceRune reports whether a rune should be collapsed to a space.
func isWhitespaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
}
