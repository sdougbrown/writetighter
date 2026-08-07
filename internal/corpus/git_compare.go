// Package corpus builds a comparison corpus from a declared Git revision and
// counts term occurrences for corpus-novelty comparison.
//
// It uses read-only Git commands only and never modifies the working tree.
// The comparison corpus is built once per lint invocation and is not persisted.
package corpus

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/codecomment"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
)

// GitCompare holds term counts from the requested Git comparison revision and
// change counts across all selected input documents. A nil GitCompare in a
// check.RunContext means --git-compare was not requested, so the novelty
// checker must abstain.
type GitCompare struct {
	// Revision is the resolved Git SHA for the comparison revision.
	Revision string

	// TermCounts maps normalized terms to their occurrence count in the
	// comparison corpus at Revision.
	TermCounts map[string]int

	// PhraseCounts maps normalized multiword phrases (2–3 words) to their
	// occurrence count in the comparison corpus at Revision.
	PhraseCounts map[string]int

	// ChangeTermCounts maps normalized terms to their occurrence count across
	// all selected input documents. Computed by the app before running checkers.
	ChangeTermCounts map[string]int

	// ChangePhraseCounts maps normalized phrases to their occurrence count across
	// all selected input documents.
	ChangePhraseCounts map[string]int
}

// IsIdentifier reports whether token looks like a code identifier (camelCase,
// PascalCase, ALLCAPS acronym of 2–5 chars, or digit-bearing) when examined
// in its original case. This MUST be called before lowercasing.
func IsIdentifier(token string) bool {
	if len(token) == 0 {
		return false
	}
	hasDigit := false
	uppercaseCount := 0
	for i, r := range token {
		if unicode.IsDigit(r) {
			hasDigit = true
		}
		if unicode.IsUpper(r) {
			uppercaseCount++
			if i > 0 {
				return true // uppercase not at position 0 → camelCase or PascalCase
			}
		}
	}
	if hasDigit {
		return true
	}
	n := utf8.RuneCountInString(token)
	return uppercaseCount >= 2 && uppercaseCount <= 5 && uppercaseCount == n
}

// IsURLOrPath reports whether token looks like a URL, file path, issue ID,
// version string, or vendor package name.
func IsURLOrPath(token string) bool {
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "ftp://") {
		return true
	}
	if strings.Contains(token, "/") && strings.Contains(token, ".") {
		return true
	}
	if strings.HasPrefix(token, "#") {
		return true
	}
	// version strings: v2.1.0, 2.1.0
	dotIdx := strings.IndexByte(token, '.')
	if dotIdx > 0 {
		before := token[:dotIdx]
		if before == "v" {
			return true
		}
		// Check if 'before' starts with v+digits or just digits
		if len(before) >= 1 && unicode.IsDigit(rune(before[0])) {
			return true
		}
		if len(before) >= 2 && before[0] == 'v' && unicode.IsDigit(rune(before[1])) {
			return true
		}
	}
	if strings.HasPrefix(token, "@") {
		return true
	}
	return false
}

// FoldUnicode lowercases a string via Unicode case folding.
// It is exported so other packages can share a single implementation rather
// than duplicating their own lowercase helpers.
func FoldUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// isProseExt reports whether ext is a prose file extension supported by
// document segmentMarkdown.
func isProseExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".md", ".markdown", ".txt", ".html", ".htm":
		return true
	}
	return false
}

// isCodeFile reports whether the file is a supported code-comment language.
func isCodeFile(path string) bool {
	_, ok := codecomment.DetectLanguage(path)
	return ok
}

// isEligible reports whether a file at the comparison revision belongs in the
// corpus.
func isEligible(path string) bool {
	ext := filepath.Ext(path)
	return isProseExt(ext) || isCodeFile(path)
}

// ExtractText extracts eligible text from file content. For code files,
// it extracts comment text via codecomment.Extract. For prose files,
// it extracts prose segments via document segmentation.
func ExtractText(path, content string) string {
	if isCodeFile(path) {
		language, _ := codecomment.DetectLanguage(path)
		catalog, err := codecomment.Extract(path, language, []byte(content))
		if err != nil {
			return ""
		}
		var parts []string
		for _, c := range catalog.Comments {
			parts = append(parts, c.Text)
		}
		return strings.Join(parts, " ")
	}
	// Prose file: use document segmentation to extract prose segments.
	doc, err := document.FromPlainText(content, path, "")
	if err != nil {
		return ""
	}
	var parts []string
	for _, seg := range doc.Segments {
		if seg.Type == document.SegmentProse {
			parts = append(parts, seg.Text)
		}
	}
	return strings.Join(parts, " ")
}

// Tokenize splits text into lowercased word tokens. Hyphens within words
// are preserved (countable-items stays as one token). Punctuation is
// stripped from edges.
func Tokenize(text string) []string {
	var tokens []string
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			i++
			continue
		}
		start := i
		for i < len(runes) {
			r := runes[i]
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '\'' || r == '\u2019' {
				i++
				continue
			}
			break
		}
		raw := string(runes[start:i])
		// Strip leading/trailing non-alphanumeric (hyphens, apostrophes at edges)
		raw = strings.TrimFunc(raw, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if raw == "" {
			continue
		}
		tokens = append(tokens, FoldUnicode(raw))
	}
	return tokens
}

// ExtractPhrases extracts bounded 2–3 word phrases from text, lowercased.
func ExtractPhrases(text string) []string {
	// Clean: keep only word chars, hyphens, and spaces
	var b strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == ' ' || r == '\n' || r == '\t' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	words := strings.Fields(b.String())
	var phrases []string
	for i := range words {
		for n := 2; n <= 3 && i+n <= len(words); n++ {
			phrase := FoldUnicode(strings.Join(words[i:i+n], " "))
			phrases = append(phrases, phrase)
		}
	}
	return phrases
}

// CountTerms counts token and phrase occurrences in text. Returns (tokenCounts, phraseCounts).
func CountTerms(text string) (map[string]int, map[string]int) {
	tokenCounts := make(map[string]int)
	for _, t := range Tokenize(text) {
		tokenCounts[t]++
	}
	phraseCounts := make(map[string]int)
	for _, p := range ExtractPhrases(text) {
		phraseCounts[p]++
	}
	return tokenCounts, phraseCounts
}

// IsExcluded reports whether a term should be excluded from novelty comparison.
// projectTerms and dict are used for precedence checks.
func IsExcluded(term string, projectTerms []config.TermEntry, dict *profile.Dictionary) bool {
	// Stopwords are checked via the stopword set in the checker, not here.
	// This function checks only structural exclusions and policy exclusions.
	if IsIdentifier(term) {
		return true
	}
	if IsURLOrPath(term) {
		return true
	}
	// Project term precedence
	if len(projectTerms) > 0 {
		if e := profile.ResolveTerm(dict, projectTerms, term); e != nil {
			return true
		}
	} else if dict != nil {
		if dict.Lookup(term) != nil {
			return true
		}
	}
	if utf8.RuneCountInString(term) < 3 {
		return true
	}
	return false
}

// IsPhraseExcluded reports whether any word in a phrase is excluded.
func IsPhraseExcluded(phrase string, projectTerms []config.TermEntry, dict *profile.Dictionary) bool {
	words := strings.Fields(phrase)
	for _, w := range words {
		if IsExcluded(w, projectTerms, dict) {
			return true
		}
	}
	return false
}

// BuildGitCompare builds a comparison corpus from the given revision in the
// repository root. It uses read-only Git commands only.
func BuildGitCompare(repoDir, revision string) (*GitCompare, error) {
	// Resolve the comparison revision to a full SHA.
	sha, err := gitRevParse(repoDir, revision)
	if err != nil {
		return nil, fmt.Errorf("comparison revision %q: %w", revision, err)
	}

	// Verify the comparison revision exists as a commit.
	if err := gitCatFileExists(repoDir, sha); err != nil {
		return nil, fmt.Errorf("comparison revision %q: %w", revision, err)
	}

	// List files at the comparison revision.
	files, err := gitLsTree(repoDir, sha)
	if err != nil {
		return nil, fmt.Errorf("listing comparison files: %w", err)
	}

	// Filter to eligible files and sort.
	var eligible []string
	for _, f := range files {
		if isEligible(f) {
			eligible = append(eligible, f)
		}
	}
	sort.Strings(eligible)

	// Count terms across all eligible files.
	termCounts := make(map[string]int)
	phraseCounts := make(map[string]int)
	for _, f := range eligible {
		content, err := gitShow(repoDir, sha, f)
		if err != nil {
			continue // skip unreadable files
		}
		text := ExtractText(f, content)
		tc, pc := CountTerms(text)
		for t, c := range tc {
			termCounts[t] += c
		}
		for p, c := range pc {
			phraseCounts[p] += c
		}
	}

	return &GitCompare{
		Revision:     sha,
		TermCounts:   termCounts,
		PhraseCounts: phraseCounts,
	}, nil
}

// FindRepoRoot returns the Git toplevel for the given path, or an error if
// the path is not inside a Git repository.
func FindRepoRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	dir := abs
	if !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a Git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRevParse(repoDir, revision string) (string, error) {
	cmd := exec.Command("git", "rev-parse", revision)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCatFileExists(repoDir, sha string) error {
	cmd := exec.Command("git", "cat-file", "-e", sha)
	cmd.Dir = repoDir
	_, err := cmd.Output()
	return err
}

func gitLsTree(repoDir, sha string) ([]string, error) {
	cmd := exec.Command("git", "ls-tree", "-r", "--name-only", sha)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func gitShow(repoDir, sha, path string) (string, error) {
	cmd := exec.Command("git", "show", sha+":"+path)
	cmd.Dir = repoDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show %s:%s: %w (%s)", sha, path, err, stderr.String())
	}
	return string(out), nil
}
