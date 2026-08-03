package reference

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/limits"
)

// TestCollectExplicitFile reads a single file reference and checks content.
func TestCollectExplicitFile(t *testing.T) {
	dir := t.TempDir()
	content := "Hello, world!"
	path := filepath.Join(dir, "greeting.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(pack.Entries))
	}
	e := pack.Entries[0]
	if e.Content != content {
		t.Errorf("content = %q, want %q", e.Content, content)
	}
	if e.InputBytes != len(content) {
		t.Errorf("InputBytes = %d, want %d", e.InputBytes, len(content))
	}
	if e.IncludedBytes != len(content) {
		t.Errorf("IncludedBytes = %d, want %d", e.IncludedBytes, len(content))
	}
	if e.DisplayPath != path {
		t.Errorf("DisplayPath = %q, want %q", e.DisplayPath, path)
	}
	if !pack.Complete {
		t.Error("pack should be complete")
	}
}

// TestCollectDirectory walks a directory with mixed files, verifying filtering and sorting.
func TestCollectDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create files with mixed extensions.
	files := map[string]string{
		"readme.md":          "# Readme",
		"main.go":            "package main",
		"data.json":          `{"key": "value"}`,
		"notes.txt":          "Some notes",
		"secret.pem":         "should not appear",
		".hidden/config.txt": "should not appear",
		"sub/deep/file.py":   "print('hello')",
	}
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Expected: readme.md, main.go, data.json, notes.txt, sub/deep/file.py
	// .hidden is skipped, secret.pem is skipped.
	// Sorted lexically within directory traversal.
	if len(pack.Entries) == 0 {
		t.Fatal("expected entries from directory")
	}

	// Verify entries are sorted lexically by display path.
	for i := 1; i < len(pack.Entries); i++ {
		if pack.Entries[i-1].DisplayPath >= pack.Entries[i].DisplayPath {
			t.Errorf("entries not sorted at index %d: %q >= %q",
				i, pack.Entries[i-1].DisplayPath, pack.Entries[i].DisplayPath)
		}
	}

	// Check that .env and .pem files are excluded.
	for _, e := range pack.Entries {
		if strings.Contains(e.DisplayPath, ".pem") {
			t.Errorf("found excluded extension in entry: %s", e.DisplayPath)
		}
		if strings.Contains(e.DisplayPath, ".hidden") {
			t.Errorf("found hidden dir entry: %s", e.DisplayPath)
		}
	}

	// Check we have the expected files.
	displayPaths := make(map[string]bool)
	for _, e := range pack.Entries {
		displayPaths[e.DisplayPath] = true
	}
	base := filepath.Base(dir)
	expected := []string{
		filepath.Join(base, "data.json"),
		filepath.Join(base, "main.go"),
		filepath.Join(base, "notes.txt"),
		filepath.Join(base, "readme.md"),
		filepath.Join(base, "sub", "deep", "file.py"),
	}
	for _, exp := range expected {
		if !displayPaths[exp] {
			t.Errorf("expected entry %q not found", exp)
		}
	}
}

// TestCollectDeduplicatesPaths verifies that the same file via different paths
// is included only once.
func TestCollectDeduplicatesPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Add the same file twice via different relative paths.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{path, rel}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry (deduplicated), got %d", len(pack.Entries))
	}
}

// TestCollectRejectsSymlinks verifies that explicitly-passed symlinks are
// rejected at top level.
func TestCollectRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("real"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}

	// Explicit symlink should error.
	_, err := Collect([]string{linkPath}, nil)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

// TestCollectWarnsOnSymlinksInDir verifies symlinks inside directories are
// skipped but reported via pack.Warnings so the exclusion is not silent.
func TestCollectWarnsOnSymlinksInDir(t *testing.T) {
	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("real"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}

	// Directory containing symlink should succeed, skipping the symlink.
	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Only real.txt should be included.
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry (symlink skipped), got %d", len(pack.Entries))
	}
	// The skipped symlink must be surfaced as a warning.
	if len(pack.Warnings) != 1 {
		t.Fatalf("expected 1 warning for skipped symlink, got %d: %v", len(pack.Warnings), pack.Warnings)
	}
	if !strings.Contains(pack.Warnings[0], "link.txt") {
		t.Fatalf("warning %q should name the skipped symlink", pack.Warnings[0])
	}
}

// TestCollectSkipsHiddenDirectories verifies that hidden directories are skipped.
func TestCollectSkipsHiddenDirectories(t *testing.T) {
	dir := t.TempDir()

	visible := filepath.Join(dir, "visible.txt")
	if err := os.WriteFile(visible, []byte("visible"), 0o600); err != nil {
		t.Fatal(err)
	}

	hiddenDir := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hiddenFile := filepath.Join(hiddenDir, "secret.txt")
	if err := os.WriteFile(hiddenFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range pack.Entries {
		if strings.Contains(e.DisplayPath, ".hidden") {
			t.Errorf("found hidden directory entry: %s", e.DisplayPath)
		}
	}
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry (visible only), got %d", len(pack.Entries))
	}
}

// TestCollectSkipsGit verifies that .git directories are skipped.
func TestCollectSkipsGit(t *testing.T) {
	dir := t.TempDir()

	visible := filepath.Join(dir, "readme.md")
	if err := os.WriteFile(visible, []byte("visible"), 0o600); err != nil {
		t.Fatal(err)
	}

	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(gitDir, "config")
	if err := os.WriteFile(gitFile, []byte("[core]"), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range pack.Entries {
		if strings.Contains(e.DisplayPath, ".git") {
			t.Errorf("found .git entry: %s", e.DisplayPath)
		}
	}
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry (visible only), got %d", len(pack.Entries))
	}
}

// TestCollectRejectsInvalidUTF8 verifies that files with invalid UTF-8 cause an error.
func TestCollectRejectsInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 'a'}, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Collect([]string{path}, nil)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("expected UTF-8 error, got %v", err)
	}
}

// TestCollectRejectsOversizedFile verifies that files exceeding MaxFileBytes cause an error.
func TestCollectRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	data := make([]byte, limits.MaxFileBytes+1)
	for i := range data {
		data[i] = 'a' + byte(i%26)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Collect([]string{path}, nil)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too large error, got %v", err)
	}
}

// TestCollectExcludesSourcePaths verifies that paths matching source paths are excluded.
func TestCollectExcludesSourcePaths(t *testing.T) {
	dir := t.TempDir()

	ref := filepath.Join(dir, "reference.md")
	if err := os.WriteFile(ref, []byte("reference content"), 0o600); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("source content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The source file should be excluded from references.
	pack, err := Collect([]string{dir, ref}, []string{source})
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range pack.Entries {
		if e.SourcePath == source {
			t.Errorf("source path %q should be excluded", source)
		}
	}
	// reference.md should still be included.
	found := false
	for _, e := range pack.Entries {
		if e.SourcePath == ref {
			found = true
			break
		}
	}
	if !found {
		t.Error("reference.md should be included")
	}
}

// TestCollectSecretFileError verifies that explicitly specifying a .env file returns an error.
func TestCollectSecretFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("SECRET=value"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Collect([]string{path}, nil)
	if err == nil || !strings.Contains(err.Error(), "secret/binary") {
		t.Fatalf("expected secret/binary error, got %v", err)
	}
}

// TestCollectRejectsSSHPrivateKeyFilename verifies that common SSH private-key
// filenames without extensions (id_rsa, id_dsa, id_ecdsa, id_ed25519) are
// rejected as secret files even when explicitly provided as reference paths.
func TestCollectRejectsSSHPrivateKeyFilename(t *testing.T) {
	for _, name := range []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte("PRIVATE KEY CONTENT"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Collect([]string{path}, nil)
			if err == nil || !strings.Contains(err.Error(), "secret/binary") {
				t.Fatalf("expected secret/binary error for %s, got %v", name, err)
			}
		})
	}
}

// TestCollectSkipsSecretDirEntry verifies that .env files in a directory are silently skipped.
func TestCollectSkipsSecretDirEntry(t *testing.T) {
	dir := t.TempDir()

	normal := filepath.Join(dir, "readme.md")
	if err := os.WriteFile(normal, []byte("# Readme"), 0o600); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("SECRET=value"), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range pack.Entries {
		if strings.Contains(e.DisplayPath, ".env") {
			t.Errorf("found .env entry: %s", e.DisplayPath)
		}
	}
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry (.env skipped), got %d", len(pack.Entries))
	}
}

// TestCollectHTMLProjection verifies that HTML files have visible text extracted
// while markdown files have raw content.
func TestCollectHTMLProjection(t *testing.T) {
	dir := t.TempDir()

	htmlPath := filepath.Join(dir, "page.html")
	htmlContent := "<html><body><h1>Title</h1><p>Hello world.</p></body></html>"
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	mdPath := filepath.Join(dir, "doc.md")
	mdContent := "# Title\n\nHello world."
	if err := os.WriteFile(mdPath, []byte(mdContent), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var htmlEntry, mdEntry *Entry
	for i, e := range pack.Entries {
		if strings.HasSuffix(e.SourcePath, ".html") {
			htmlEntry = &pack.Entries[i]
		}
		if strings.HasSuffix(e.SourcePath, ".md") {
			mdEntry = &pack.Entries[i]
		}
	}

	if htmlEntry == nil {
		t.Fatal("expected HTML entry")
	}
	if mdEntry == nil {
		t.Fatal("expected markdown entry")
	}

	// HTML should have visible text only (stripped tags).
	if strings.Contains(htmlEntry.Content, "<html>") || strings.Contains(htmlEntry.Content, "<body>") {
		t.Errorf("HTML entry should not contain raw HTML tags; got %q", htmlEntry.Content)
	}
	if !strings.Contains(htmlEntry.Content, "Title") || !strings.Contains(htmlEntry.Content, "Hello world") {
		t.Errorf("HTML entry should contain visible text; got %q", htmlEntry.Content)
	}

	// HTML IncludedBytes should be less than InputBytes (after stripping).
	if htmlEntry.IncludedBytes >= htmlEntry.InputBytes && htmlEntry.InputBytes > 0 {
		t.Errorf("HTML IncludedBytes (%d) should be < InputBytes (%d)",
			htmlEntry.IncludedBytes, htmlEntry.InputBytes)
	}

	// Markdown should have raw content preserved (no projection for .md).
	if mdEntry.Content != mdContent {
		t.Errorf("markdown content = %q, want %q", mdEntry.Content, mdContent)
	}
}

// TestPackRender verifies that the rendered output has correct XML-like tags.
func TestPackRender(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "guide.md")
	if err := os.WriteFile(path1, []byte("Guide content"), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{path1}, nil)
	if err != nil {
		t.Fatal(err)
	}

	rendered := pack.Render()
	t.Logf("Rendered:\n%s", rendered)

	// Check opening tag.
	if !strings.Contains(rendered, "<reference file=") {
		t.Error("rendered output missing opening <reference> tag")
	}
	// Check closing tag.
	if !strings.Contains(rendered, "</reference>") {
		t.Error("rendered output missing closing </reference> tag")
	}
	// Check content.
	if !strings.Contains(rendered, "Guide content") {
		t.Error("rendered output missing file content")
	}
	// Check display path appears in file= attribute.
	if !strings.Contains(rendered, "guide.md") {
		t.Error("rendered output missing display path")
	}
}

// TestPackRenderCachesResult verifies repeated Render calls reuse the cached
// rendering (avoiding redundant strings.Builder allocations in budget planning).
func TestPackRenderCachesResult(t *testing.T) {
	// Build the pack directly (bypassing Collect, which pre-warms the cache while
	// computing IncludedBytes) so the uncached first call is actually exercised.
	pack := &Pack{Entries: []Entry{
		{DisplayPath: "guide.md", Content: "Guide content"},
		{DisplayPath: "refs/glossary.json", Content: "glossary body"},
	}}
	if pack.renderedReady {
		t.Fatal("freshly constructed pack should not have a cached render")
	}

	first := pack.Render()
	if first == "" {
		t.Fatal("Render on a fresh pack should build output")
	}
	if !pack.renderedReady {
		t.Fatal("Render should mark the cache as ready after the first call")
	}
	if !strings.Contains(first, "<reference file=\"guide.md\">") ||
		!strings.Contains(first, "<reference file=\"refs/glossary.json\">") ||
		!strings.Contains(first, "</reference>") {
		t.Fatal("rendered output missing reference wrappers")
	}

	second := pack.Render()
	if first != second {
		t.Fatal("Render returned different output across calls")
	}
}

// TestCollectSetsIncludedBytes verifies Collect computes IncludedBytes from the
// rendered size and that the value is stable across repeated Render calls.
func TestCollectSetsIncludedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guide.md")
	if err := os.WriteFile(path, []byte("Guide content"), 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := Collect([]string{path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pack.IncludedBytes != len(pack.Render()) {
		t.Fatalf("IncludedBytes=%d does not match rendered length %d", pack.IncludedBytes, len(pack.Render()))
	}
}

// TestCollectOrdering verifies that files are sorted lexically regardless
// of filesystem order.
func TestCollectOrdering(t *testing.T) {
	dir := t.TempDir()

	// Create files out of order.
	for _, name := range []string{"zeta.txt", "alpha.txt", "gamma.txt", "beta.txt"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		filepath.Join(filepath.Base(dir), "alpha.txt"),
		filepath.Join(filepath.Base(dir), "beta.txt"),
		filepath.Join(filepath.Base(dir), "gamma.txt"),
		filepath.Join(filepath.Base(dir), "zeta.txt"),
	}
	if len(pack.Entries) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(pack.Entries))
	}
	for i, exp := range expected {
		if pack.Entries[i].DisplayPath != exp {
			t.Errorf("entry %d: DisplayPath = %q, want %q", i, pack.Entries[i].DisplayPath, exp)
		}
	}
}

// TestCollectEmptyPath verifies that empty input produces an empty pack.
func TestCollectEmptyPath(t *testing.T) {
	pack, err := Collect(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(pack.Entries))
	}
	if !pack.Complete {
		t.Error("empty pack should be complete")
	}
}

// TestCollectSingleFileBeyondMaxAggregate verifies that aggregate limits are enforced.
func TestCollectSingleFileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")
	// Create a file that's within MaxFileBytes but too large for aggregate.
	data := make([]byte, limits.MaxFileBytes) // exactly MaxFileBytes is within per-file limit
	for i := range data {
		data[i] = 'a' + byte(i%26)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Single file of MaxFileBytes should fit within MaxAggregateBytes.
	pack, err := Collect([]string{path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(pack.Entries))
	}
	if !pack.Complete {
		t.Error("pack should be complete")
	}
}

// TestCollectErrNoFit verifies ErrNoFit is properly formatted.
func TestCollectErrNoFit(t *testing.T) {
	err := &ErrNoFit{Available: 1000, Required: 5000}
	msg := err.Error()
	if !strings.Contains(msg, "1000") || !strings.Contains(msg, "5000") {
		t.Errorf("ErrNoFit.Error() = %q, want available and required", msg)
	}
}

// TestCollectRejectsBinaryFile verifies that files with null bytes are rejected.
func TestCollectRejectsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(path, []byte("text\x00binary"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Collect([]string{path}, nil)
	if err == nil || !strings.Contains(err.Error(), "null bytes") {
		t.Fatalf("expected null byte error, got %v", err)
	}
}

// TestCollectDirectoryNested verifies nested directory traversal with sorting.
func TestCollectDirectoryNested(t *testing.T) {
	dir := t.TempDir()

	// Create a nested structure.
	subDirs := []string{
		"a/aa",
		"a/ab",
		"b",
	}
	for _, sd := range subDirs {
		full := filepath.Join(dir, sd)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		"a/aa/note.md": "aa note",
		"a/ab/note.md": "ab note",
		"b/note.md":    "b note",
		"root.md":      "root",
	}
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := Collect([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}

	base := filepath.Base(dir)
	expected := []string{
		filepath.Join(base, "a", "aa", "note.md"),
		filepath.Join(base, "a", "ab", "note.md"),
		filepath.Join(base, "b", "note.md"),
		filepath.Join(base, "root.md"),
	}
	if len(pack.Entries) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(pack.Entries))
	}
	for i, exp := range expected {
		if pack.Entries[i].DisplayPath != exp {
			t.Errorf("entry %d: DisplayPath = %q, want %q", i, pack.Entries[i].DisplayPath, exp)
		}
	}
}

// TestCollectHTMLWithScriptStyle verifies that script and style content is hidden.
func TestCollectHTMLWithScriptStyle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	htmlContent := `<html><head><script>alert("x")</script><style>body{color:red}</style></head><body><p>Visible text.</p></body></html>`
	if err := os.WriteFile(path, []byte(htmlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(pack.Entries))
	}

	e := pack.Entries[0]
	if strings.Contains(e.Content, "alert") {
		t.Errorf("script content should be hidden; got %q", e.Content)
	}
	if strings.Contains(e.Content, "body{color") {
		t.Errorf("style content should be hidden; got %q", e.Content)
	}
	if !strings.Contains(e.Content, "Visible text") {
		t.Errorf("visible text should be present; got %q", e.Content)
	}
}

// TestCollectMultipleExplicitFiles checks collecting multiple explicit files.
func TestCollectMultipleExplicitFiles(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "a.txt")
	path2 := filepath.Join(dir, "b.go")
	path3 := filepath.Join(dir, "c.md")
	if err := os.WriteFile(path1, []byte("file a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path2, []byte("package b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path3, []byte("# file c"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Pass in non-lexical order to verify output is sorted.
	pack, err := Collect([]string{path3, path1, path2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(pack.Entries))
	}

	// Entries should be sorted by DisplayPath.
	if !strings.HasSuffix(pack.Entries[0].DisplayPath, "a.txt") {
		t.Errorf("first entry should be a.txt, got %s", pack.Entries[0].DisplayPath)
	}
	if !strings.HasSuffix(pack.Entries[1].DisplayPath, "b.go") {
		t.Errorf("second entry should be b.go, got %s", pack.Entries[1].DisplayPath)
	}
	if !strings.HasSuffix(pack.Entries[2].DisplayPath, "c.md") {
		t.Errorf("third entry should be c.md, got %s", pack.Entries[2].DisplayPath)
	}
}

// TestRenderMultipleEntries checks rendering of multiple entries.
func TestRenderMultipleEntries(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "a.md")
	path2 := filepath.Join(dir, "b.md")
	if err := os.WriteFile(path1, []byte("Content A"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path2, []byte("Content B"), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := Collect([]string{path1, path2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(pack.Entries))
	}

	rendered := pack.Render()

	// Check both entries appear.
	if !strings.Contains(rendered, "Content A") || !strings.Contains(rendered, "Content B") {
		t.Error("rendered output missing content")
	}

	// Check there are two reference blocks.
	count := strings.Count(rendered, "<reference file=")
	if count != 2 {
		t.Errorf("expected 2 <reference> blocks, got %d", count)
	}
	countClose := strings.Count(rendered, "</reference>")
	if countClose != 2 {
		t.Errorf("expected 2 </reference> blocks, got %d", countClose)
	}
}
