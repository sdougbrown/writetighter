package writetighter

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestVersionJSON(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/writetighter", "version", "--json")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty version output")
	}
}

func TestProfileVerify(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/writetighter", "profile", "verify", "software-docs-en@0.1.0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("profile verify failed: %s", string(out))
	}
}

func TestOfflineStdin(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/writetighter", "check", "--stdin", "--kind", "pr", "--format", "json")
	cmd.Stdin = bytes.NewBufferString("Create the file.\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check failed: %s", string(out))
	}
}
