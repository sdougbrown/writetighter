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
		{"bracketIndexId", true}, // camelCase
		{"NodeFilter", true},     // PascalCase
		{"API", true},            // ALLCAPS acronym (3)
		{"EI", true},             // ALLCAPS acronym (2)
		{"hello", false},         // ordinary word
		{"Hello", false},         // sentence-initial capital only
		{"level1", true},         // digit-bearing
		{"case", false},          // ordinary word
		{"bracket-mesh", false},  // hyphenated ordinary words
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
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "hyphenated words",
			input: "The bracket-mesh overgrid provides the factory keys.",
			want:  []string{"the", "bracket-mesh", "overgrid", "provides", "the", "factory", "keys"},
		},
		{
			name:  "re-derives",
			input: "The overgrid re-derives metadata.",
			want:  []string{"the", "overgrid", "re-derives", "metadata"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokens := Tokenize(tc.input)
			if len(tokens) != len(tc.want) {
				t.Errorf("Tokenize(%q): got %d tokens, want %d; got %v", tc.input, len(tokens), len(tc.want), tokens)
			}
			for i, tok := range tokens {
				if i >= len(tc.want) || tok != tc.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tc.input, i, tok, tc.want[i])
				}
			}
		})
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

func TestIsProseExt(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".md", true},
		{".markdown", true},
		{".txt", true},
		{".html", true},
		{".htm", true},
		{".go", false},
		{".js", false},
		{".json", false},
		{".yaml", false},
	}
	for _, tc := range tests {
		got := isProseExt(tc.ext)
		if got != tc.want {
			t.Errorf("isProseExt(%q) = %v, want %v", tc.ext, got, tc.want)
		}
	}
}

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"src/main.ts", true},
		{"src/index.tsx", true},
		{"main.go", true},
		{"README.md", false},
		{"data.json", false},
		{"Makefile", false},
	}
	for _, tc := range tests {
		got := isCodeFile(tc.path)
		if got != tc.want {
			t.Errorf("isCodeFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestExtractPhrases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "all 2- and 3-word phrases",
			input: "one two three four",
			want:  []string{"one two", "one two three", "two three", "two three four", "three four"},
		},
		{
			name:  "3-word phrases for short input",
			input: "a b c d",
			want:  []string{"a b", "a b c", "b c", "b c d", "c d"},
		},
		{
			name:  "single word excluded",
			input: "hello",
			want:  nil,
		},
		{
			name:  "no 4-word phrases",
			input: "one two three four five",
			want: []string{"one two", "one two three", "two three", "two three four",
				"three four", "three four five", "four five"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			phrases := ExtractPhrases(tc.input)
			if len(phrases) != len(tc.want) {
				t.Errorf("ExtractPhrases(%q): got %d phrases, want %d; got %v", tc.input, len(phrases), len(tc.want), phrases)
				return
			}
			for i, p := range phrases {
				if p != tc.want[i] {
					t.Errorf("ExtractPhrases(%q)[%d] = %q, want %q", tc.input, i, p, tc.want[i])
				}
			}
		})
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

func TestBuildGitCompare(t *testing.T) {
	// Create a temp git repo
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@test.test")
	runGit(t, dir, "config", "user.name", "Test")

	// Write gitCompare file with a comment using "sorter"
	gitCompareContent := "// The sorter reads the tagline from the camera.\n"
	os.WriteFile(filepath.Join(dir, "sorter.ts"), []byte(gitCompareContent), 0644)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "gitCompare")

	// Get the gitCompare SHA
	rev := runGitOutput(t, dir, "rev-parse", "HEAD")

	// Write changed file with a coined term
	changedContent := "// The spinotron drives the warp core.\n// The spinotron re-derives metadata.\n"
	os.WriteFile(filepath.Join(dir, "sorter.ts"), []byte(changedContent), 0644)

	// BuildGitCompare gitCompare
	gitCompare, err := BuildGitCompare(dir, rev)
	if err != nil {
		t.Fatal(err)
	}

	// "sorter" should be in gitCompare with count 1
	if gitCompare.TermCounts["sorter"] == 0 {
		t.Error("expected 'sorter' to be in gitCompare term counts")
	}
	if gitCompare.TermCounts["tagline"] == 0 {
		t.Error("expected 'tagline' to be in gitCompare term counts")
	}
	// "spinotron" should NOT be in gitCompare
	if gitCompare.TermCounts["spinotron"] > 0 {
		t.Error("expected 'spinotron' to NOT be in gitCompare")
	}
	// Revision should be the full SHA
	if gitCompare.Revision != rev {
		t.Errorf("expected revision %s, got %s", rev, gitCompare.Revision)
	}
}

func TestBuildGitCompareMissingRevision(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@test.test")
	runGit(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "test.ts"), []byte("// hello\n"), 0644)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "gitCompare")

	_, err := BuildGitCompare(dir, "nonexistent-revision")
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
