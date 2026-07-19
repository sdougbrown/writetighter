package document

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestCollectInputsFileTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.txt")
	data := make([]byte, 5*1024*1024+1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := CollectInputs([]string{path}, false)
	if err == nil || !strings.Contains(err.Error(), "file too large") {
		t.Fatalf("expected file too large error, got %v", err)
	}
}

func TestCollectInputsSymlinkRejected(t *testing.T) {
	realPath := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(realPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	symPath := filepath.Join(t.TempDir(), "link.txt")
	if err := os.Symlink(realPath, symPath); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}
	_, err := CollectInputs([]string{symPath}, false)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestCollectInputsAggregateLimit(t *testing.T) {
	// Test: two files close to 5 MiB each, aggregate < 25 MiB should work.
	path1 := filepath.Join(t.TempDir(), "a.txt")
	path2 := filepath.Join(t.TempDir(), "b.txt")
	data := make([]byte, 3*1024*1024)
	if err := os.WriteFile(path1, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path2, data, 0o600); err != nil {
		t.Fatal(err)
	}
	// Total is 6 MiB < 25, should work
	_, err := CollectInputs([]string{path1, path2}, false)
	if err != nil {
		t.Fatalf("expected OK, got %v", err)
	}
	// Add a third file that pushes aggregate over 25 MiB
	path3 := filepath.Join(t.TempDir(), "c.txt")
	bigData := make([]byte, 22*1024*1024) // 22 MiB > 5 MiB limit -> file too large
	if err := os.WriteFile(path3, bigData, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = CollectInputs([]string{path1, path2, path3}, false)
	if err == nil {
		t.Fatal("expected error from file too large")
	}
	t.Logf("got expected error: %v", err)
}

func TestCollectInputsAggregateViaDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create multiple files inside directory whose total exceeds 25 MiB
	for i := 0; i < 6; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		data := make([]byte, 5*1024*1024) // 5 MiB each (at limit)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := CollectInputs([]string{dir}, false)
	if err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("expected aggregate limit error, got %v", err)
	}
	t.Logf("got expected error: %v", err)
}

func TestCollectInputsExplicitFileNotIsAllowed(t *testing.T) {
	// Explicit file paths should be read regardless of extension.
	// Even non-standard extensions should be read as plain text.
	path := filepath.Join(t.TempDir(), "notes.log")
	if err := os.WriteFile(path, []byte("some text"), 0o600); err != nil {
		t.Fatal(err)
	}
	docs, err := CollectInputs([]string{path}, false)
	if err != nil {
		t.Fatalf("explicit file should be read: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
}

// Issue 6: Aggregate limit across two directories (centralized accounting).
// Each file must be under 5 MiB but combined over 25 MiB.
func TestCollectInputsAggregateAcrossTwoDirectories(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	data4 := make([]byte, 4*1024*1024) // 4 MiB each, 4*7=28 MiB > 25 MiB
	for i := 0; i < 4; i++ {
		if err := os.WriteFile(filepath.Join(dir1, fmt.Sprintf("a%d.md", i)), data4, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(dir2, fmt.Sprintf("b%d.md", i)), data4, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := CollectInputs([]string{dir1, dir2}, false)
	if err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("expected aggregate limit error across two dirs, got %v", err)
	}
}

func TestCollectInputsAggregateDirPlusFile(t *testing.T) {
	dir := t.TempDir()
	data4 := make([]byte, 4*1024*1024) // 4 MiB
	for i := 0; i < 6; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.md", i)), data4, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// 6*4 = 24 MiB from dir, add 4 MiB from file = 28 MiB > 25 MiB
	path2 := filepath.Join(t.TempDir(), "also_big.md")
	if err := os.WriteFile(path2, data4, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := CollectInputs([]string{dir, path2}, false)
	if err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("expected aggregate limit error for dir+file, got %v", err)
	}
}

func TestCollectInputsBoundedFileRead(t *testing.T) {
	// A file > 5 MiB should be rejected at the read stage regardless of stat.
	path := filepath.Join(t.TempDir(), "big.md")
	data := make([]byte, 5*1024*1024+1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := CollectInputs([]string{path}, false)
	if err == nil || !strings.Contains(err.Error(), "file too large") {
		t.Fatalf("expected file too large error, got %v", err)
	}
}

func TestCollectInputsBoundedFileReadWithinLimit(t *testing.T) {
	// A file exactly 5 MiB should be accepted.
	path := filepath.Join(t.TempDir(), "ok.md")
	data := make([]byte, 5*1024*1024)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := CollectInputs([]string{path}, false)
	if err != nil {
		t.Fatalf("expected OK for 5 MiB file, got %v", err)
	}
}
