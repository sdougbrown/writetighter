package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

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

func TestRunPrompt(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	if got := run([]string{"prompt", "--kind", "code-comment", "--format", "json"}); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
	_ = w.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "code-comment" {
		t.Fatalf("unexpected prompt payload: %v", payload)
	}
}

func TestStrictFlagAndEnumValidation(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--wat"}, {"lint", "--stdin", "--kind", "blog"},
		{"lint", "--stdin", "--format", "yaml"}, {"explain", "--wat", "CORE.SENTENCE_LENGTH"},
		{"prompt", "--kind", "message"}, {"prompt", "--format", "yaml"}, {"prompt", "extra"},
		{"profile", "list", "--wat"},
	} {
		if got := run(args); got != 2 {
			t.Fatalf("%v: got %d", args, got)
		}
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

func TestLintAcceptsDirectText(t *testing.T) {
	if got := run([]string{"lint", "--text", "Short direct text.", "--format", "json"}); got != 0 {
		t.Fatalf("expected direct text to lint successfully, got %d", got)
	}
	if got := run([]string{"lint", "--text", "text", "file.md"}); got != 2 {
		t.Fatalf("expected --text and path conflict, got %d", got)
	}
	if got := run([]string{"lint", "--text", "text", "--stdin"}); got != 2 {
		t.Fatalf("expected --text and stdin conflict, got %d", got)
	}
}

func TestLintAcceptedFlags(t *testing.T) {
	// lint accepts paths before flags.
	if got := run([]string{"lint", "missing.txt"}); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func TestConfigRejectsArguments(t *testing.T) {
	if got := run([]string{"config", "extra"}); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
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

func TestReviseAcceptsDirectTextFlag(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	if got := run([]string{"revise", "--text", "Short direct text."}); got != 2 {
		t.Fatalf("expected missing model config exit 2 after accepting --text, got %d", got)
	}
	if got := run([]string{"revise", "--text", "text", "file.md"}); got != 2 {
		t.Fatalf("expected --text and path conflict, got %d", got)
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

func TestCheckCommandDoesNotExist(t *testing.T) {
	if got := run([]string{"check", "--stdin"}); got != 2 {
		t.Fatalf("expected removed check command to return exit 2, got %d", got)
	}
}
