package main

import "testing"

func TestRunCheckUsage(t *testing.T) {
	if got := run([]string{"check", "--stdin", "foo"}); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := run([]string{"check"}); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := run([]string{"check", "--require-llm", "foo"}); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestRunVersion(t *testing.T) {
	if got := run([]string{"version", "--json"}); got != 0 {
		t.Fatalf("got %d", got)
	}
}

func TestRunExplain(t *testing.T) {
	// explain with a valid rule ID now works (resolves embedded profile, prints)
	if got := run([]string{"explain", "CORE.TEST"}); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}
