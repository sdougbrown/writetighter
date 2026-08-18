package llm

import (
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/check"
	"github.com/sdougbrown/writetighter/internal/guidance"
)

// TestLintPrincipleFallbackConsistency guards the mapping table: every key must
// be a registered deterministic lint rule and every value a valid revision
// principle, so we never coerce to an ID the validator would reject.
func TestLintPrincipleFallbackConsistency(t *testing.T) {
	for ruleID, principleID := range lintPrincipleFallback {
		if check.Get(ruleID) == nil {
			t.Errorf("fallback key %q is not a registered lint rule", ruleID)
		}
		if !guidance.IsPrincipleID(principleID) {
			t.Errorf("fallback value %q is not a valid revision principle", principleID)
		}
	}
}

// TestSanitizePrincipleIDsCoercesLintRule asserts that a model echoing a lint
// rule ID (the reported CORE.UNEXPANDED_ABBREV case) is coerced to its closest
// revision principle rather than failing.
func TestSanitizePrincipleIDsCoercesLintRule(t *testing.T) {
	got, err := sanitizePrincipleIDs([]string{"CORE.UNEXPANDED_ABBREV"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "CORE.EXPLICIT_RELATIONSHIPS" {
		t.Fatalf("expected coerce to EXPLICIT_RELATIONSHIPS, got %v", got)
	}
}

// TestSanitizePrincipleIDsDedupesCoercion asserts that an echoed lint rule ID
// next to the principle it coerces to collapses to a single entry.
func TestSanitizePrincipleIDsDedupesCoercion(t *testing.T) {
	got, err := sanitizePrincipleIDs([]string{"CORE.UNEXPANDED_ABBREV", "CORE.EXPLICIT_RELATIONSHIPS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "CORE.EXPLICIT_RELATIONSHIPS" {
		t.Fatalf("expected single deduped EXPLICIT_RELATIONSHIPS, got %v", got)
	}
}

// TestSanitizePrincipleIDsDropsUnmappedLintRule asserts that a lint rule ID with
// no mapping is dropped without error; a genuine principle alongside it survives.
func TestSanitizePrincipleIDsDropsUnmappedLintRule(t *testing.T) {
	got, err := sanitizePrincipleIDs([]string{"CORE.CONTRACTION", "CORE.SHORT_SENTENCE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "CORE.SHORT_SENTENCE" {
		t.Fatalf("expected SHORT_SENTENCE to survive, got %v", got)
	}
}

// TestSanitizePrincipleIDsRejectsUnknown asserts that an invented ID is still a
// contract violation and is rejected (not silently dropped).
func TestSanitizePrincipleIDsRejectsUnknown(t *testing.T) {
	if _, err := sanitizePrincipleIDs([]string{"CORE.FOOBAR"}); err == nil || !strings.Contains(err.Error(), "unknown principle id") {
		t.Fatalf("expected unknown principle id error, got: %v", err)
	}
}

// TestValidateReviseResponseCoercesNewFindingLevels a full response where every
// finding[0] echoes a lint rule ID: it should succeed and coerce, never fail the
// batch.
func TestValidateReviseResponseCoercesLintRuleEcho(t *testing.T) {
	raw := `{"findings":[{"kind":"rewrite","source_text":"please use SSL","source_range":{"start":0,"end":14},"principle_ids":["CORE.UNEXPANDED_ABBREV"],"reason":"expand the abbreviation on first use","replacement":"use TLS/SSL","confidence":0.8}]}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if len(resp.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(resp.Revisions))
	}
	if got := resp.Revisions[0].PrincipleIDs; len(got) != 1 || got[0] != "CORE.EXPLICIT_RELATIONSHIPS" {
		t.Fatalf("expected coerced EXPLICIT_RELATIONSHIPS, got %v", got)
	}
	if resp.DiscardedFindings != 0 {
		t.Fatalf("expected 0 discarded findings, got %d", resp.DiscardedFindings)
	}
}

// TestValidateReviseResponseDropsUnmappedOnlyFinding asserts that a finding whose
// only principle is an unmapped lint rule is dropped (not a batch failure) and
// counted as discarded, while a sibling valid finding still survives.
func TestValidateReviseResponseDropsUnmappedOnlyFinding(t *testing.T) {
	raw := `{"findings":[` +
		`{"kind":"rewrite","source_text":"please wait","source_range":{"start":0,"end":11},"principle_ids":["CORE.BANNED_MODAL"],"reason":"modal weakens the instruction","replacement":"wait","confidence":0.8},` +
		`{"kind":"rewrite","source_text":"too long","source_range":{"start":12,"end":20},"principle_ids":["CORE.SHORT_SENTENCE"],"reason":"shorten","replacement":"short","confidence":0.9}]}`
	resp, err := ValidateReviseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if len(resp.Revisions) != 1 {
		t.Fatalf("expected 1 surviving revision, got %d", len(resp.Revisions))
	}
	if resp.DiscardedFindings != 1 {
		t.Fatalf("expected 1 discarded finding, got %d", resp.DiscardedFindings)
	}
}

// TestSanitizePrincipleIDsCoercesStyleRules asserts that the style-guide lint
// rules introduced in software-docs-en@0.6.0 coerce to their mapped
// principles instead of failing the batch.
func TestSanitizePrincipleIDsCoercesStyleRules(t *testing.T) {
	cases := []struct{ rule, want string }{
		{"CORE.TIME_ANCHOR", "CORE.TIMELESS_PROSE"},
		{"CORE.GERUND_HEADING", "CORE.ACTIVE_DIRECT_VOICE"},
		{"CORE.SEQUENTIAL_BULLET", "CORE.CAUSAL_ORDER"},
	}
	for _, c := range cases {
		got, err := sanitizePrincipleIDs([]string{c.rule})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.rule, err)
		}
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("%s: expected coerce to %s, got %v", c.rule, c.want, got)
		}
	}
}

// TestSanitizePrincipleIDSDropsStyleRulesWithoutMapping asserts that style
// rules without a semantic principle mapping are dropped, not rejected.
func TestSanitizePrincipleIDSDropsStyleRulesWithoutMapping(t *testing.T) {
	got, err := sanitizePrincipleIDs([]string{"CORE.EXCLAMATION", "CORE.HEADING_CASE", "CORE.SHORT_SENTENCE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "CORE.SHORT_SENTENCE" {
		t.Fatalf("expected only SHORT_SENTENCE to survive, got %v", got)
	}
}
