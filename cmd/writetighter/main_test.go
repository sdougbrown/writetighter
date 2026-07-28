package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

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
	t.Run("flags before rule", func(t *testing.T) {
		old := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		os.Stdout = w
		defer func() { os.Stdout = old }()

		if got := run([]string{"explain", "--format", "json", "CORE.SENTENCE_LENGTH"}); got != 0 {
			t.Fatalf("expected exit 0, got %d", got)
		}
		_ = w.Close()
		var out bytes.Buffer
		if _, err := io.Copy(&out, r); err != nil {
			t.Fatalf("copy stdout: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		if payload["id"] != "CORE.SENTENCE_LENGTH" {
			t.Fatalf("unexpected id: %v", payload["id"])
		}
	})
}

func TestStrictFlagAndEnumValidation(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--wat"}, {"check", "--stdin", "--kind", "blog"},
		{"check", "--stdin", "--format", "yaml"}, {"explain", "--wat", "CORE.SENTENCE_LENGTH"},
		{"profile", "list", "--wat"},
	} {
		if got := run(args); got != 2 {
			t.Fatalf("%v: got %d", args, got)
		}
	}
}

func TestCheckAcceptsFlagsAfterPath(t *testing.T) {
	if got := run([]string{"check", "missing.txt", "--format", "wat"}); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestLintCommand(t *testing.T) {
	if got := run([]string{"lint"}); got != 2 {
		t.Fatalf("expected exit 2 for missing input, got %d", got)
	}
	if got := run([]string{"lint", "--stdin", "foo"}); got != 2 {
		t.Fatalf("expected exit 2 for stdin+path, got %d", got)
	}
	if got := run([]string{"lint", "--stdin", "--llm"}); got != 2 {
		t.Fatalf("expected exit 2 because lint has no LLM flags, got %d", got)
	}
}

func TestLintAcceptedFlags(t *testing.T) {
	// lint should accept the same positional+flags as check (minus --llm).
	if got := run([]string{"lint", "missing.txt"}); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestReviseCommand(t *testing.T) {
	if got := run([]string{"revise"}); got != 2 {
		t.Fatalf("expected exit 2 for missing input, got %d", got)
	}
	if got := run([]string{"revise", "--stdin", "foo"}); got != 2 {
		t.Fatalf("expected exit 2 for stdin+path, got %d", got)
	}
}

func TestReviseNoLLMConfigFails(t *testing.T) {
	// Without any config file, revise should fail with a clear message about
	// missing LLM configuration.
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	if got := run([]string{"revise", "missing.txt"}); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
}

func TestCheckIsBackwardCompatible(t *testing.T) {
	// The check command still handles the same flags.
	if got := run([]string{"check", "--stdin"}); got != 0 {
		t.Fatalf("expected exit 0 for stdin with no data, got %d", got)
	}
	// Check with --llm but no model should fail with config error (exit 2).
	if got := run([]string{"check", "--stdin", "--llm"}); got != 2 {
		t.Fatalf("expected exit 2 for --llm without model, got %d", got)
	}
}
