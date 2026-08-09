package check

import (
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/report"
)

// Feedback-driven regression tests for CORE.NOUN_STACK and
// CORE.UNEXPANDED_ABBREV synthesized from the PR #1590 / #1669 before-after
// samples.

func runNounStack(t *testing.T, text, kind string) []report.Finding {
	t.Helper()
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	ctx.Document.Kind = kind
	f, err := Get("CORE.NOUN_STACK").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestNounStackDoesNotCrossClauseBoundaries(t *testing.T) {
	// "Case size changed" sits before an em dash; the window must never span it.
	text := "Case size changed \u2014 rebuild the CaseMeasurements before release."
	for _, f := range runNounStack(t, text, "code-comment") {
		if strings.Contains(f.Evidence, "Case size changed") {
			t.Fatalf("noun stack must not cross an em dash: %s", f.Evidence)
		}
	}
	// Semicolon, colon, and en dash are clause boundaries too.
	for _, sep := range []string{";", ":", "\u2013"} {
		text := "fixture build overrides stay partial" + sep + " keep the counts"
		for _, f := range runNounStack(t, text, "code-comment") {
			if strings.Contains(f.Evidence, "overrides stay partial") || strings.Contains(f.Evidence, "build overrides stay") {
				t.Fatalf("noun stack crossed a %q boundary: %s", sep, f.Evidence)
			}
		}
	}
}

func TestNounStackRejectsVerbalWindows(t *testing.T) {
	// Windows that end or embed a frequent finite verb are clauses, not noun stacks.
	// Legitimate stacks elsewhere in the same sentence may still be flagged; the
	// assertion is that the clause-shaped window itself is never the evidence.
	cases := []struct {
		text      string
		forbidden string // substrings that must not appear in any finding evidence
	}{
		{"Item ID field parsed, then checked.", "field parsed"},
		{"Numeric item ID leading zeros stripped when read.", "zeros stripped"},
		{"raw mis-scanned barcode registered twice.", "barcode registered"},
		{"Level-1 item class id sent on the wire.", "class id sent"},
		{"unit conversions use the new case spec.", "conversions use"},
		{"newly added fragment field reads only once.", "field reads"},
		{"fixture fishery's build overrides stay partial now.", "overrides stay partial"},
		{"cast slipping past the first guard is a bug.", "cast slipping"},
		{"See countableItemFactory required keys keep the shape.", "keys keep"},
	}
	for _, c := range cases {
		for _, f := range runNounStack(t, c.text, "description") {
			if strings.Contains(strings.ToLower(f.Evidence), strings.ToLower(c.forbidden)) {
				t.Fatalf("clause window %q should not be the stack evidence (forbidden %q): %s", c.text, c.forbidden, f.Evidence)
			}
		}
	}
}

func TestNounStackPreservesPlainLanguageStacks(t *testing.T) {
	// Real noun-stack findings in plain prose still fire.
	ctx := &RunContext{Document: testDoc("The assumptions-side entry wrappers are configured."), Profile: testProfile()}
	ctx.Document.Kind = "description"
	findings, _ := Get("CORE.NOUN_STACK").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("plain-language noun stack should still be detected")
	}
	// A participle modifier in a non-final position must not kill the stack.
	ctx = &RunContext{Document: testDoc("Left-aligned text labels wrap cleanly."), Profile: testProfile()}
	ctx.Document.Kind = "description"
	findings, _ = Get("CORE.NOUN_STACK").Run(ctx)
	if len(findings) == 0 || !strings.Contains(findings[0].Evidence, "Left-aligned text labels") {
		t.Fatalf("participle modifier must not kill the stack: %v", findings)
	}
}

func TestNounStackSuppressesIdentifiersInCodeComments(t *testing.T) {
	// Identifier/proper-name-heavy stacks are the comment doing its job in
	// code-adjacent prose; the abbreviation rule owns the acronym jargon.
	texts := []string{
		"batch path uses primaryCategoryId now.",
		"EI item writes skip Apollo cache normalization.",
		"Item ID field parsed later.",
	}
	for _, text := range texts {
		for _, f := range runNounStack(t, text, "code-comment") {
			t.Fatalf("identifier-heavy stack %q should be suppressed in code comments: %s", text, f.Evidence)
		}
	}
}

func TestNounStackFeedbackNoiseSweep(t *testing.T) {
	// The 19 noisy findings from PR #1590 in code-comment kind: tokens from the
	// clause/identifier failures must not appear in any finding evidence. The
	// handful of genuine plain-language stacks may remain.
	text := strings.Join([]string{
		"Item ID field parsed.", "Numeric item ID leading zeros stripped.",
		"raw mis-scanned barcode registered.", "Level-1 item class id sent.",
		"EI item writes skip Apollo cache normalization.", "Localized display name.",
		"iPad's manual-add path keeps type checks.", "synthetic Manually Added category.",
		"batch path uses primaryCategoryId.", "Case size changed rebuild the set.",
		"cast slipping past the guard.", "unit conversions use the spec.",
		"Client-side flag item.", "new case spec.",
		"level-1 item class chosen.", "newly added fragment field reads.",
		"new fragment field.", "fixture fishery's build overrides stay partial.",
		"See countableItemFactory required keys keep.",
	}, "\n")
	findings := runNounStack(t, text, "code-comment")
	forbidden := []string{"parsed", "stripped", "registered", "sent", "skip",
		"primaryCategoryId", "Case size", "cast slipping", "use the spec",
		"chosen", "reads", "keep", "stay partial", "countableItemFactory",
		"iPad", "EI item"}
	for _, f := range findings {
		ev := strings.ToLower(f.Evidence)
		for _, bad := range forbidden {
			if strings.Contains(ev, bad) {
				t.Fatalf("noise finding survived: %s (contains %q)", f.Evidence, bad)
			}
		}
	}
	// The genuine plain-language stacks that remain must still be surfaced:
	// this is the precision payoff — a handful of real noun stacks, not 19.
	remaining := strings.ToLower(func() string {
		var b strings.Builder
		for _, f := range findings {
			b.WriteString(f.Evidence)
			b.WriteByte('\n')
		}
		return b.String()
	}())
	for _, keep := range []string{"localized display name", "new fragment field", "client-side flag item", "synthetic manually added category"} {
		if !strings.Contains(remaining, keep) {
			t.Fatalf("genuine stack %q should still be detected in code-comment prose: %s", keep, remaining)
		}
	}
	if len(findings) > 6 {
		t.Fatalf("expected a small high-precision finding set, got %d: %s", len(findings), remaining)
	}
}

func TestUnexpandedAbbrevFindsBareAbbreviation(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Match the other EI item writes: copy the guard."), Profile: testProfile()}
	ctx.Document.Kind = "code-comment"
	findings, err := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if err != nil || len(findings) != 1 {
		t.Fatalf("expected one EI finding, got %d, err=%v", len(findings), err)
	}
	if !strings.Contains(findings[0].Evidence, "EI") {
		t.Fatalf("expected EI evidence, got %q", findings[0].Evidence)
	}
}

func TestUnexpandedAbbrevSkipsCommonTechnicalAbbreviations(t *testing.T) {
	text := "The API returns an ID for the URL; use HTTPS only."
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("common technical abbreviations must not be flagged: %v", findings)
	}
	// File extensions used as bare names in code prose are excluded too.
	ext := "The TSX and JSX files compile; the JSON and YAML schema stays in git."
	ctx = &RunContext{Document: testDoc(ext), Profile: testProfile()}
	if findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx); len(findings) != 0 {
		t.Fatalf("extension-name abbreviations must not be flagged: %v", findings)
	}
}

func TestUnexpandedAbbrevExpansionInEitherOrder(t *testing.T) {
	for _, text := range []string{
		"The EI (Ending Inventory) item writes match.",
		"The Ending Inventory (EI) item writes match.",
		"Use NPE (Null Pointer Exception) detection here.",
	} {
		ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
		findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
		if len(findings) != 0 {
			t.Fatalf("expanded abbreviation %q still flagged: %v", text, findings)
		}
	}
}

func TestUnexpandedAbbrevExpansionAnywhereInDocumentSuppresses(t *testing.T) {
	// A definition in an earlier comment suppresses later bare uses.
	text := "EI (Ending Inventory) is populated on save.\n\nThe EI sync path handles retries."
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("in-document expansion should suppress later use: %v", findings)
	}
}

func TestUnexpandedAbbrevReportsFirstOccurrenceOnce(t *testing.T) {
	text := "EI and EI again from the EI module."
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding for repeated abbreviation, got %d: %v", len(findings), findings)
	}
}

func TestUnexpandedAbbrevSkipsCodeSpans(t *testing.T) {
	text := "Run the `EI` probe against the service."
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("abbreviation inside inline code must not be flagged: %v", findings)
	}
}

func TestUnexpandedAbbrevIgnoresDottedAndSingleLetterTokens(t *testing.T) {
	text := "The N.P.E. guard fires only here."
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("dotted single-letter tokens must not be flagged: %v", findings)
	}
}

func TestUnexpandedAbbrevIgnoresIdentifierSubstrings(t *testing.T) {
	text := "NPEHandler passes HTTPServer to countEIValue."
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("identifier substrings must not be flagged as prose abbreviations: %v", findings)
	}
}

func TestNounStackGardenPathAmbiguity(t *testing.T) {
	// The social-media example: a determiner followed by a verb/noun homograph
	// at the head of a content-word run should produce an ambiguity finding.
	findings := runNounStack(t, "Give the prune logic real attention.", "description")
	if len(findings) == 0 {
		t.Fatal("expected garden-path noun stack finding")
	}
	if !strings.Contains(findings[0].Evidence, "ambiguous noun stack") {
		t.Fatalf("expected ambiguous noun stack evidence, got %q", findings[0].Evidence)
	}
	if !strings.Contains(findings[0].Message, "structurally ambiguous") {
		t.Fatalf("expected ambiguity message, got %q", findings[0].Message)
	}
}

func TestNounStackGardenPathNotFlaggedForKnownNoun(t *testing.T) {
	// "database" is not a gardenPathHead, so even after a determiner the
	// stack should be flagged as a regular noun stack, not ambiguous.
	findings := runNounStack(t, "The database connection pool is configured.", "description")
	if len(findings) == 0 {
		t.Fatal("expected noun stack finding")
	}
	if strings.Contains(findings[0].Evidence, "ambiguous noun stack") {
		t.Fatalf("known-noun stack should not be marked ambiguous: %s", findings[0].Evidence)
	}
	if !strings.Contains(findings[0].Evidence, "noun stack") {
		t.Fatalf("expected plain noun stack evidence, got %q", findings[0].Evidence)
	}
}

func TestNounStackGardenPathNoDeterminer(t *testing.T) {
	// A run starting with a gardenPathHead but NOT preceded by a determiner
	// should be flagged as a regular noun stack, not ambiguous.
	findings := runNounStack(t, "Build queue size matters.", "description")
	hasAmbiguous := false
	for _, f := range findings {
		if strings.Contains(f.Evidence, "ambiguous noun stack") {
			hasAmbiguous = true
			break
		}
	}
	if hasAmbiguous {
		t.Fatalf("stack without preceding determiner must not be marked ambiguous: %v", findings)
	}
}

func TestNounStackGardenPathClauseBoundaryResetsPrecedingToken(t *testing.T) {
	// A gardenPathHead after a clause boundary should NOT be marked ambiguous
	// because the preceding token is reset (not a determiner).
	text := "Run the config check \u2014 build queue size matters."
	for _, f := range runNounStack(t, text, "description") {
		if strings.Contains(f.Evidence, "ambiguous noun stack") {
			t.Fatalf("stack after clause boundary must not be ambiguous: %s", f.Evidence)
		}
	}
}

func TestNounStackGardenPathPrecedingTokenNotDeterminer(t *testing.T) {
	// A stack where preceding token is a preposition, not a determiner.
	findings := runNounStack(t, "For build queue size, set the limit.", "description")
	hasAmbiguous := false
	for _, f := range findings {
		if strings.Contains(f.Evidence, "ambiguous noun stack") {
			hasAmbiguous = true
			break
		}
	}
	if hasAmbiguous {
		t.Fatalf("stack preceded by preposition must not be marked ambiguous: %v", findings)
	}
}

func TestNounStackGardenPathShortStackNotFlagged(t *testing.T) {
	// 2-word stacks should never be flagged regardless of ambiguity.
	findings := runNounStack(t, "The build queue is full.", "description")
	if len(findings) != 0 {
		t.Fatalf("2-word stack should not be flagged at threshold 3: %v", findings)
	}
}

func TestNounStackGardenPathSendIsInNounRunFinalVerbs(t *testing.T) {
	// "send" is in nounRunFinalVerbs, so "the data send" would be treated
	// as a clause, not a noun stack, and should not be flagged at all.
	findings := runNounStack(t, "The data send is complete.", "description")
	// "send" is in nounRunFinalVerbs, so the run is rejected entirely.
	if len(findings) != 0 {
		t.Fatalf("'send' is a nounRunFinalVerb -- stack should be rejected: %v", findings)
	}
}

func TestUnexpandedAbbrevHonorsRulePolicy(t *testing.T) {
	p := testProfile()
	for i := range p.Rules.Rules {
		if p.Rules.Rules[i].ID == "CORE.UNEXPANDED_ABBREV" {
			p.Rules.Rules[i].Enforcement = "enforced"
			p.Rules.Rules[i].Severity = "warning"
		}
	}
	ctx := &RunContext{Document: testDoc("The EI guard is missing."), Profile: p}
	findings, _ := Get("CORE.UNEXPANDED_ABBREV").Run(ctx)
	if len(findings) != 1 || findings[0].Enforcement != "enforced" || findings[0].Severity != "warning" {
		t.Fatalf("expected enforced/warning policy, got %v", findings)
	}
}
