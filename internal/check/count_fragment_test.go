package check

import (
	"testing"

	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

func runCountFragment(t *testing.T, text string) []report.Finding {
	t.Helper()
	ctx := &RunContext{Document: testDoc(text), Profile: testProfile()}
	findings, err := Get("CORE.COUNT_FRAGMENT").Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return findings
}

func TestCountFragmentFiresOnCanonicalExample(t *testing.T) {
	findings := runCountFragment(t, "The rig is small. Six legs. One GPU, one server build, one sampler, one question set.")
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Enforcement != "candidate" || f.Severity != "info" {
		t.Fatalf("expected candidate/info, got %s/%s", f.Enforcement, f.Severity)
	}
	if f.Message != "Repeated count-led fragments. Consider a sentence or list if these describe one setup." {
		t.Fatalf("unexpected message: %s", f.Message)
	}
}

func TestCountFragmentSecondExample(t *testing.T) {
	findings := runCountFragment(t, "Deployment is tightly scoped. Three constraints. One region, one writer, one owner.")
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(findings))
	}
}

func TestCountFragmentAbstainsWithoutCardinalClass(t *testing.T) {
	p := testProfile()
	// A dictionary with no word classes at all: the checker must abstain,
	// not fire blind. (wordClassSet is built during Validate.)
	p.Dict = &profile.Dictionary{
		FormatVersion: 2,
		Entries:       []profile.Entry{{Term: "deprecated term", Status: profile.StatusDiscouraged, Alternatives: []string{"preferred term"}, Reason: "use preferred term", PartsOfSpeech: []string{"noun"}}},
	}
	_ = p.Dict.Validate()
	ctx := &RunContext{Document: testDoc("Six legs. One GPU, one server build, one sampler, one question set."), Profile: p}
	findings, err := Get("CORE.COUNT_FRAGMENT").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected checker to abstain without cardinal_number class, got %d", len(findings))
	}
}

func TestCountFragmentDoesNotFire(t *testing.T) {
	cases := map[string]string{
		"complete clauses":      "Six legs were tested. One GPU served all requests.",
		"isolated inventory":    "One GPU, one server build, one sampler, one question set.",
		"single-item inventory": "Six legs. One GPU served all requests.",
		"deferred-noun pair":    "Five of those six rows are one cluster. The sixth is not.",
		"list items":            "- Six legs. One GPU, one server build, one sampler, one question set.\n",
		"blockquote":            "> Six legs. One GPU, one server build, one sampler, one question set.\n",
		"quoted speech":         `The spec reads, "Six legs. One GPU, one server build, one sampler, one question set."`,
		"table rows":            "| 6 | legs |\n|---|---|\n| one GPU | one sampler |",
	}
	for name, text := range cases {
		findings := runCountFragment(t, text)
		if len(findings) != 0 {
			t.Errorf("%s: expected no findings, got %d (%s)", name, len(findings), findings[0].Evidence)
		}
	}
}
