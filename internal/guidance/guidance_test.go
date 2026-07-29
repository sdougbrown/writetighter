package guidance

import "testing"

func TestKindsHaveCompleteIndependentGuidance(t *testing.T) {
	for _, kind := range []string{KindDescription, KindProcedure, KindPR, KindCodeComment} {
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

func TestCodeCommentInheritsDescriptionSentenceLimit(t *testing.T) {
	if got := SentenceLimitParameter(KindCodeComment); got != "description_max_words" {
		t.Fatalf("code-comment parameter = %q", got)
	}
	if got := SentenceLimitParameter(KindPR); got != "pr_max_words" {
		t.Fatalf("pr parameter = %q", got)
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
