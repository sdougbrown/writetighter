package document

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestFromFileSimpleMarkdown(t *testing.T) {
	doc, err := FromFile(filepath.Join(repoRoot(t), "testdata", "documents", "simple.md"), "pr")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source == "" || len(doc.Segments) == 0 {
		t.Fatal("missing document data")
	}
}

func TestFromFileSimpleText(t *testing.T) {
	doc, err := FromFile(filepath.Join(repoRoot(t), "testdata", "documents", "simple.txt"), "pr")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Segments) != 1 || doc.Segments[0].Type != SegmentProse {
		t.Fatalf("unexpected segments: %#v", doc.Segments)
	}
}

func TestCollectInputsDirectory(t *testing.T) {
	docs, err := CollectInputs([]string{filepath.Join(repoRoot(t), "testdata", "documents")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("expected docs")
	}
}

func TestInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(path, []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FromFile(path, "pr"); err == nil {
		t.Fatal("expected error")
	}
}
