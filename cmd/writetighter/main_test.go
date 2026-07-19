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
