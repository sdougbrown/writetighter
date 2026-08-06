package corpus

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsIdentifier(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"bracketIndexId", true},  // camelCase
		{"NodeFilter", true},        // PascalCase
		{"API", true},                 // ALLCAPS acronym (3)
		{"EI", true},                  // ALLCAPS acronym (2)
		{"hello", false},              // ordinary word
		{"Hello", false},              // sentence-initial capital only
		{"level1", true},              // digit-bearing
		{"case", false},               // ordinary word
		{"bracket-mesh", false},    // hyphenated ordinary words
		{"", false},
	}
	for _, tc := range tests {
		got := IsIdentifier(tc.token)
		if got != tc.want {
			t.Errorf("IsIdentifier(%q) = %v, want %v", tc.token, got, tc.want)
		}
	}
}

func TestIsURLOrPath(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"https://example.com", true},
		{"http://foo.bar/baz", true},
		{"#1590", true},
		{"v2.1.0", true},
		{"2.1.0", true},
		{"@fictional/widget-lib", true},
		{"src/components/Button.tsx", true},
		{"hello", false},
		{"overgrid", false},
	}
	for _, tc := range tests {
		got := IsURLOrPath(tc.token)
		if got != tc.want {
			t.Errorf("IsURLOrPath(%q) = %v, want %v", tc.token, got, tc.want)
		}
	}
}

func TestTokenize(t *testing.T) {
	tokens := Tokenize("The bracket-mesh overgrid provides the factory keys.")
	// Should include "bracket-mesh" as a single hyphenated token
	found := false
	for _, tok := range tokens {
		if tok == "bracket-mesh" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'bracket-mesh' in tokens, got %v", tokens)
	}
}

func TestCountTerms(t *testing.T) {
	text := "The overgrid provides keys. The overgrid re-derives metadata."
	tc, pc := CountTerms(text)
	if tc["overgrid"] != 2 {
		t.Errorf("expected overgrid count=2, got %d", tc["overgrid"])
	}
	if tc["provides"] != 1 {
		t.Errorf("expected provides count=1, got %d", tc["provides"])
	}
	// Should have "the overgrid" phrase
	if pc["the overgrid"] != 2 {
		t.Errorf("expected 'the overgrid' phrase count=2, got %d", pc["the overgrid"])
	}
}

func TestExtractTextFromCodeFile(t *testing.T) {
	// A .ts file with comments
	content := `// The bracket-mesh overgrid provides keys.
// The overgrid re-derives metadata.
export const x = 1;`
	text := ExtractText("test.ts", content)
	if !contains(text, "bracket-mesh overgrid") {
		t.Errorf("expected comment text to include 'bracket-mesh overgrid', got: %q", text)
	}
	if contains(text, "export") {
		t.Errorf("expected code to be excluded, got: %q", text)
	}
}

func TestExtractTextFromProseFile(t *testing.T) {
	content := "# Title\n\nThe bracket-mesh overgrid provides keys.\n\n`code span`\n"
	text := ExtractText("test.md", content)
	if !contains(text, "bracket-mesh overgrid") {
		t.Errorf("expected prose text to include 'bracket-mesh overgrid', got: %q", text)
	}
	if contains(text, "code span") {
		t.Errorf("expected inline code to be excluded, got: %q", text)
	}
}

func TestEnumerate(t *testing.T) {
	// Create a temp git repo
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@test.test")
	runGit(t, dir, "config", "user.name", "Test")

	// Write baseline file with a comment using "sorter"
	baselineContent := "// The sorter reads the tagline from the camera.\n"
	os.WriteFile(filepath.Join(dir, "sorter.ts"), []byte(baselineContent), 0644)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "baseline")

	// Get the baseline SHA
	rev := runGitOutput(t, dir, "rev-parse", "HEAD")

	// Write changed file with a coined term
	changedContent := "// The spinotron drives the warp core.\n// The spinotron re-derives metadata.\n"
	os.WriteFile(filepath.Join(dir, "sorter.ts"), []byte(changedContent), 0644)

	// Enumerate baseline
	baseline, err := Enumerate(dir, rev)
	if err != nil {
		t.Fatal(err)
	}

	// "sorter" should be in baseline with count 1
	if baseline.TermCounts["sorter"] == 0 {
		t.Error("expected 'sorter' to be in baseline term counts")
	}
	if baseline.TermCounts["tagline"] == 0 {
		t.Error("expected 'tagline' to be in baseline term counts")
	}
	// "spinotron" should NOT be in baseline
	if baseline.TermCounts["spinotron"] > 0 {
		t.Error("expected 'spinotron' to NOT be in baseline")
	}
	// Revision should be the full SHA
	if baseline.Revision != rev {
		t.Errorf("expected revision %s, got %s", rev, baseline.Revision)
	}
}

func TestEnumerateMissingRevision(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@test.test")
	runGit(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "test.ts"), []byte("// hello\n"), 0644)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "baseline")

	_, err := Enumerate(dir, "nonexistent-revision")
	if err == nil {
		t.Fatal("expected error for missing revision")
	}
}

func TestFindRepoRoot(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")

	subdir := filepath.Join(dir, "src", "components")
	os.MkdirAll(subdir, 0755)
	filePath := filepath.Join(subdir, "Button.tsx")
	os.WriteFile(filePath, []byte("// test\n"), 0644)

	root, err := FindRepoRoot(filePath)
	if err != nil {
		t.Fatal(err)
	}
	// FindRepoRoot should return the git toplevel
	if root != dir {
		t.Errorf("expected repo root %s, got %s", dir, root)
	}
}

func TestFindRepoRootNonGit(t *testing.T) {
	dir := t.TempDir()
	_, err := FindRepoRoot(filepath.Join(dir, "test.ts"))
	if err == nil {
		t.Fatal("expected error for non-Git directory")
	}
}

// helpers

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)[:len(out)-1] // trim trailing newline
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}