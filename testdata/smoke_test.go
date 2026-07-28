package writetighter

import (
	"bytes"
	"os/exec"
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
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func TestVersionJSON(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/writetighter", "version", "--json")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty version output")
	}
}

func TestProfileVerify(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/writetighter", "profile", "verify", "software-docs-en@0.2.0")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("profile verify failed: %s", string(out))
	}
}

func TestLintStdin(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/writetighter", "lint", "--stdin", "--kind", "pr", "--format", "json")
	cmd.Dir = repoRoot(t)
	cmd.Stdin = bytes.NewBufferString("Create the file.\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lint failed: %s", string(out))
	}
}

func TestOfflineStdin(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/writetighter", "check", "--stdin", "--kind", "pr", "--format", "json")
	cmd.Dir = repoRoot(t)
	cmd.Stdin = bytes.NewBufferString("Create the file.\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check failed: %s", string(out))
	}
}
