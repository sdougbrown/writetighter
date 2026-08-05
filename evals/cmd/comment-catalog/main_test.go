package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunCatalogsFileByExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.ts")
	if err := os.WriteFile(path, []byte("// comment\nconst text = '/* string */';\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := run([]string{path}); got != 0 {
		t.Fatalf("run returned %d", got)
	}
}

func TestRunRequiresLanguageForStdin(t *testing.T) {
	if got := run(nil); got != 2 {
		t.Fatalf("run returned %d, want 2", got)
	}
}
