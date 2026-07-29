package report

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

func TestRenderJSON(t *testing.T) {
	var path *string
	r := &Report{SchemaVersion: 1, ToolVersion: "0.1.0", Source: SourceInfo{Kind: "pr", Path: path}, Profile: ProfileInfo{ID: "software-docs-en", Version: "0.1.0", SHA256: "placeholder"}, TermBase: TermBaseInfo{SHA256: "placeholder"}, Status: "linted", Claims: ClaimsInfo{Certification: "unknown"}, Coverage: CoverageInfo{Rules: []RuleCoverage{}}, Findings: []Finding{}}
	got, err := RenderJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "expected", "simple-lint.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("mismatch\n%s\n%s", got, string(want))
	}
}

func TestRenderHuman(t *testing.T) {
	got, err := RenderHuman(&Report{Status: "linted"})
	if err != nil || got == "" {
		t.Fatal("expected human output")
	}
}

func TestRenderAgent(t *testing.T) {
	got, err := RenderAgent(&Report{Findings: []Finding{{Severity: "warning", RuleID: "CORE.SENTENCE_LENGTH"}}})
	if err != nil || got == "" {
		t.Fatal("expected agent output")
	}
}
