package guidance

import (
	"strings"
	"testing"
)

func TestKindsHaveCompleteIndependentGuidance(t *testing.T) {
	for _, kind := range []string{KindDescription, KindProcedure, KindPR, KindCodeComment, KindReference, KindDecision, KindIncident, KindAgentInstruction, KindStatusUpdate} {
		set, err := ForKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		if set.Kind != kind || len(set.Principles) == 0 || len(set.CoreDirections) == 0 || len(set.KindDirections) == 0 {
			t.Fatalf("incomplete guidance for %q: %#v", kind, set)
		}
		set.KindDirections[0] = "mutated"
		fresh, err := ForKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.KindDirections[0] == "mutated" {
			t.Fatalf("guidance for %q shares mutable slices", kind)
		}
	}
}

func TestNewKindsInheritDescriptionSentenceLimit(t *testing.T) {
	for _, kind := range []string{KindCodeComment, KindReference, KindDecision, KindIncident, KindAgentInstruction, KindStatusUpdate} {
		if got := SentenceLimitParameter(kind); got != "description_max_words" {
			t.Fatalf("%s parameter = %q", kind, got)
		}
	}
	if got := SentenceLimitParameter(KindPR); got != "pr_max_words" {
		t.Fatalf("pr parameter = %q", got)
	}
}

func TestSpecializedKindsExposeDistinctDirections(t *testing.T) {
	checks := map[string][]string{
		KindReference:        {"accurate lookup", "defaults", "boundary conditions"},
		KindDecision:         {"considered alternatives", "tradeoffs", "engineering preferences"},
		KindIncident:         {"observed facts", "chronological order", "root-cause claim"},
		KindAgentInstruction: {"one control system", "change an agent action", "delegated responsibilities", "reliable execution", "decision points", "trigger metadata", "observable verification"},
		KindStatusUpdate:     {"operationally useful update", "observable state", "hypotheses", "next reporting point", "Do not manufacture a mechanism"},
	}
	for kind, expected := range checks {
		set, err := ForKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		joined := ""
		for _, direction := range set.KindDirections {
			joined += direction + "\n"
		}
		for _, phrase := range expected {
			if !strings.Contains(joined, phrase) {
				t.Fatalf("%s guidance missing %q: %s", kind, phrase, joined)
			}
		}
	}
}

func TestPrincipleIDsAreUniqueAndRecognized(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range PrincipleIDs() {
		if id == "" || seen[id] || !IsPrincipleID(id) {
			t.Fatalf("invalid principle id %q", id)
		}
		seen[id] = true
	}
	if IsPrincipleID("CORE.UNKNOWN") {
		t.Fatal("unknown principle was accepted")
	}
}

func TestInvalidKind(t *testing.T) {
	if ValidKind("message") {
		t.Fatal("unsupported kind was accepted")
	}
	if _, err := ForKind("message"); err == nil {
		t.Fatal("unsupported kind returned guidance")
	}
}

func TestCoreDirectionsCoverStyleGuidePractices(t *testing.T) {
	set, err := ForKind(KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, direction := range set.CoreDirections {
		joined += direction + "\n"
	}
	for _, phrase := range []string{
		"sentence case",
		"bare infinitive",
		"numbered lists for sequences",
		"ordinal numbers as words",
		"percent sign",
		"second person",
		"exclamation points",
	} {
		if !strings.Contains(joined, phrase) {
			t.Fatalf("core directions missing %q: %s", phrase, joined)
		}
	}
}

func TestTimelessProsePrincipleExists(t *testing.T) {
	set, err := ForKind(KindDescription)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range set.Principles {
		if p.ID == "CORE.TIMELESS_PROSE" {
			found = true
			if p.Direction == "" {
				t.Fatal("TIMELESS_PROSE has empty direction")
			}
		}
	}
	if !found {
		t.Fatal("CORE.TIMELESS_PROSE principle missing")
	}
}
