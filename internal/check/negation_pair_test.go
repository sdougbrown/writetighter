package check

import (
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/report"
)

func runNegationPair(t *testing.T, text string) []report.Finding {
	t.Helper()
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, err := Get("CORE.NEGATION_PAIR").Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return findings
}

func TestNegationPairFiresOnCanonicalExample(t *testing.T) {
	findings := runNegationPair(t, "It is not a wrong answer. It is a non-answer.")
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Enforcement != "candidate" || f.Severity != "info" {
		t.Fatalf("expected candidate/info, got %s/%s", f.Enforcement, f.Severity)
	}
	if !strings.Contains(f.Message, "Negative setup") {
		t.Fatalf("unexpected message: %s", f.Message)
	}
}

func TestNegationPairRangeSpansBothSentences(t *testing.T) {
	// Wrapped lines: the finding range must cover the full pair across the
	// line break, with byte offsets into the original content.
	text := "The result surprised the team. It is not a wrong answer.\nIt is a non-answer, and the distinction matters here."
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, err := Get("CORE.NEGATION_PAIR").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(findings))
	}
	r := findings[0].Range
	if r.StartLine == r.EndLine {
		t.Fatalf("expected range to span wrapped lines, got lines %d-%d", r.StartLine, r.EndLine)
	}
	if r.StartByte < 0 || r.EndByte > len(text) {
		t.Fatalf("range out of bounds: %d-%d", r.StartByte, r.EndByte)
	}
}

func TestNegationPairSecondPersonVariant(t *testing.T) {
	findings := runNegationPair(t, "This is not a repair. This is a workaround.")
	if len(findings) != 1 {
		t.Fatalf("expected one finding for 'this' variant, got %d", len(findings))
	}
}

func TestNegationPairKnownResidualFalsePositive(t *testing.T) {
	// Documented residual: a legitimate technical contrast is structurally
	// identical to the tic. The rule ships candidate/info precisely so this
	// stays advisory.
	findings := runNegationPair(t, "It is not a mutex. It is a semaphore.")
	if len(findings) != 1 {
		t.Fatalf("expected the accepted residual finding, got %d", len(findings))
	}
}

func TestNegationPairDoesNotFire(t *testing.T) {
	cases := map[string]string{
		"adjective complement": "It is not safe to retry. It is necessary to reconnect.",
		"different subjects":   "The response is not JSON. It is plain text.",
		"hedged complement":    "It is not one of them. It is the only one.",
		"long sentences":       "It is not a design that the reviewers found acceptable after three rounds of review and argument. It is a design that the reviewers accepted because every alternative had already failed.",
		"quoted speech":        `The reviewer wrote, "It is not a wrong answer. It is a non-answer."`,
		"blockquote":           "> It is not a wrong answer. It is a non-answer.\n",
		"list item":            "- It is not a wrong answer. It is a non-answer.\n",
		"negative then detail": "It is not a wrong answer. The scoring layer drops it before it reaches the comparator.",
		"no determiner":        "It is not wrong. It is worse than wrong.",
		"table row":            "| It is not a wrong answer. It is a non-answer. | x |\n|---|---|\n| a | b |",
	}
	for name, text := range cases {
		findings := runNegationPair(t, text)
		if len(findings) != 0 {
			t.Errorf("%s: expected no findings, got %d (%s)", name, len(findings), findings[0].Evidence)
		}
	}
}
