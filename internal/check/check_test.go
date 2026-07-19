package check

import (
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
)

func testProfile() *profile.Resolution {
	canon := "WriteTighter"
	dict := &profile.Dictionary{FormatVersion: 1, Entries: []profile.Entry{{Term: "deprecated term", Status: profile.StatusDiscouraged, Alternatives: []string{"preferred term"}, Reason: "use preferred term", PartsOfSpeech: []string{"noun"}}, {Term: "WriteTighter", Status: profile.StatusPreferred, PartsOfSpeech: []string{"proper noun"}, CanonicalCase: &canon}, {Term: "check-in", Status: profile.StatusPreferred, PartsOfSpeech: []string{"noun"}}}}
	_ = dict.Validate()
	return &profile.Resolution{Rules: &profile.RulesConfig{UnknownTermPolicy: "candidate", Rules: []profile.Rule{{ID: "CORE.SENTENCE_LENGTH", Enabled: true, Parameters: map[string]any{"description_max_words": 5}}, {ID: "CORE.DENSE_PARAGRAPH", Enabled: true}, {ID: "CORE.TERM_DISCOURAGED", Enabled: true}, {ID: "CORE.TERM_CASE", Enabled: true}, {ID: "CORE.TERM_UNKNOWN", Enabled: true}, {ID: "CORE.TERM_CONSISTENCY", Enabled: true}, {ID: "CORE.PROCEDURE_MULTI_ACTION", Enabled: true}}}, Dict: dict}
}

func testDoc(text string) *document.Document {
	doc, _ := document.FromReader(strings.NewReader(text), "test.md", "description")
	return doc
}

func TestSentenceLength(t *testing.T) {
	ctx := &RunContext{Document: testDoc("one two three four five six."), Profile: testProfile()}
	findings, err := Get("CORE.SENTENCE_LENGTH").Run(ctx)
	if err != nil || len(findings) == 0 {
		t.Fatal("expected sentence length finding")
	}
}

func TestTermDiscouraged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("deprecated term appears here."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected discouraged term finding")
	}
}

func TestDenseParagraph(t *testing.T) {
	ctx := &RunContext{Document: testDoc("One. Two. Three. Four.")}
	findings, _ := Get("CORE.DENSE_PARAGRAPH").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected dense paragraph finding")
	}
}

func TestTermCase(t *testing.T) {
	ctx := &RunContext{Document: testDoc("writetighter is here."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_CASE").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected term case finding")
	}
}

func TestTermUnknown(t *testing.T) {
	ctx := &RunContext{Document: testDoc("flarb is unknown."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_UNKNOWN").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected unknown term finding")
	}
}

func TestTermConsistency(t *testing.T) {
	ctx := &RunContext{Document: testDoc("check-in and checkin differ."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_CONSISTENCY").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected term consistency finding")
	}
}

func TestProcedureMultiAction(t *testing.T) {
	ctx := &RunContext{Document: testDoc("1. Do this and that.")}
	findings, _ := Get("CORE.PROCEDURE_MULTI_ACTION").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected procedure multi action finding")
	}
}
