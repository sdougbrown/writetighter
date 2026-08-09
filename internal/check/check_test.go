package check

import (
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
)

func testProfile() *profile.Resolution {
	canon := "WriteTighter"
	e1 := profile.Entry{Term: "deprecated term longer phrase", Status: profile.StatusDiscouraged, Alternatives: []string{"better phrase"}, Reason: "use better phrase", PartsOfSpeech: []string{"noun"}}
	e2 := profile.Entry{Term: "deprecated term", Status: profile.StatusDiscouraged, Alternatives: []string{"preferred term"}, Reason: "use preferred term", PartsOfSpeech: []string{"noun"}}
	e3 := profile.Entry{Term: "WriteTighter", Status: profile.StatusPreferred, PartsOfSpeech: []string{"proper noun"}, CanonicalCase: &canon}
	e4 := profile.Entry{Term: "check-in", Status: profile.StatusPreferred, PartsOfSpeech: []string{"noun"}}
	dict := &profile.Dictionary{
		FormatVersion: 2,
		Entries:       []profile.Entry{e1, e2, e3, e4},
		WordClasses:   testWordClasses(),
		BannedModalSuggestions: map[string]string{
			"should": "Custom: write 'must' if required.",
			"would":  "Custom: restructure.",
			"may":    "Custom: write 'can'.",
			"might":  "Custom: write 'can'.",
			"could":  "Custom: write 'can'.",
		},
		BannedLatinAbbrevSuggestions: map[string]string{
			"e.g.": "Custom: write 'for example'.",
			"i.e.": "Custom: write 'that is'.",
			"etc":  "Custom: name the items.",
			"etc.": "Custom: name the items.",
		},
	}
	_ = dict.Validate()
	rules := []profile.Rule{
		{ID: "CORE.SENTENCE_LENGTH", Enabled: true, Parameters: map[string]any{"description_max_words": 5}},
		{ID: "CORE.DENSE_PARAGRAPH", Enabled: true},
		{ID: "CORE.TERM_DISCOURAGED", Enabled: true},
		{ID: "CORE.TERM_CASE", Enabled: true},
		{ID: "CORE.TERM_UNKNOWN", Enabled: true},
		{ID: "CORE.TERM_CONSISTENCY", Enabled: true},
		{ID: "CORE.PROCEDURE_MULTI_ACTION", Enabled: true},
		{ID: "CORE.NOUN_STACK", Enabled: true, Parameters: map[string]any{"min_stack_length": 3}},
		{ID: "CORE.GERUND_OPENER", Enabled: true},
		{ID: "CORE.CONTRACTION", Enabled: true},
		{ID: "CORE.BANNED_MODAL", Enabled: true},
		{ID: "CORE.LATIN_ABBREV", Enabled: true},
		{ID: "CORE.UNEXPANDED_ABBREV", Enabled: true},
	}
	return &profile.Resolution{Rules: &profile.RulesConfig{UnknownTermPolicy: "candidate", Rules: rules}, Dict: dict}
}

func testDoc(text string) *document.Document {
	doc, _ := document.FromReader(strings.NewReader(text), "test.md", "description")
	return doc
}

// testWordClasses returns a WordClasses map suitable for unit tests.
// It mirrors the full set of word classes used by the embedded profile.
func testWordClasses() map[string][]string {
	return map[string][]string{
		"stopword": {
			"a", "an", "the",
			"in", "on", "at", "to", "for", "of", "with", "by", "from",
			"into", "onto", "upon", "over", "under", "through",
			"between", "among", "across", "against", "about", "as",
			"than", "per", "past", "without", "within", "during",
			"including", "via", "using", "despite", "toward",
			"towards", "behind", "beyond",
			"and", "or", "but", "nor", "yet", "so", "if", "then",
			"when", "while", "where", "because", "although", "though",
			"since", "unless", "until", "before", "after", "whether",
			"once", "whereas",
			"it", "its", "they", "them", "their", "there", "here",
			"this", "that", "these", "those", "which", "who", "whom",
			"whose", "what", "his", "her", "our", "your", "my",
			"we", "us", "you", "i", "he", "him", "she", "me",
			"some", "any", "all", "each", "both", "either", "neither",
			"every", "few", "many", "much", "more", "most", "less",
			"least", "other", "another", "same", "such", "own",
			"is", "are", "was", "were", "be", "been", "being", "am",
			"do", "does", "did", "have", "has", "had", "having",
			"will", "would", "can", "could", "should", "shall", "may",
			"might", "must",
			"not", "no", "also", "just", "only", "even", "still",
			"already", "never", "always", "often", "sometimes",
			"now", "too", "again", "very", "really", "quite", "rather",
			"however", "therefore", "thus", "hence", "indeed",
			"actually", "simply", "particularly", "especially",
			"specifically", "generally", "typically", "usually",
			"commonly", "normally", "previously", "currently",
			"subsequently", "finally", "next", "instead", "perhaps",
			"maybe", "likely", "basically", "essentially",
			"isn't", "aren't", "wasn't", "weren't", "don't", "doesn't",
			"didn't", "won't", "wouldn't", "can't", "couldn't",
			"shouldn't", "hasn't", "haven't", "hadn't",
			"it's", "they're", "we're", "you're", "that's", "there's",
			"here's", "what's", "who's", "let's",
			"one", "two", "three", "four", "five", "six",
		},
		"finite_verb": {
			"use", "uses", "used",
			"keep", "keeps", "kept",
			"stay", "stays", "stayed",
			"read", "reads",
			"match", "matches", "matched",
			"send", "sends", "sent",
			"pass", "passes", "passed",
			"set", "sets",
			"skip", "skips", "skipped",
			"run", "runs", "ran",
			"make", "makes", "made",
			"take", "takes", "took",
			"get", "gets", "got",
			"see", "sees", "saw",
			"come", "comes", "came",
			"go", "goes", "went",
			"hold", "holds", "held",
			"stand", "stands", "stood",
			"show", "shows", "showed",
			"mean", "means", "meant",
			"tell", "tells", "told",
			"put", "puts",
			"leave", "leaves", "left",
			"bring", "brings", "brought",
			"work", "works", "worked",
			"turn", "turns", "turned",
			"begin", "begins", "began", "begun",
			"call", "calls", "called",
			"return", "returns", "returned",
			"allow", "allows", "allowed",
			"follow", "follows", "followed",
			"cause", "causes", "caused",
			"change", "changes", "changed",
			"fail", "fails", "failed",
			"exist", "exists", "existed",
			"appear", "appears", "appeared",
			"remain", "remains", "remained",
			"happen", "happens", "happened",
			"occur", "occurs", "occurred",
			"become", "becomes", "became",
			"seem", "seems", "seemed",
			"slip", "slips", "slipped",
			"throw", "throws", "threw",
			"choose", "chooses", "chose", "chosen",
			"parse", "parses", "parsed",
			"strip", "strips", "stripped",
			"register", "registers", "registered",
			"resolve", "resolves", "resolved",
			"update", "updates", "updated",
			"check", "checks", "checked",
			"drop", "drops", "dropped",
			"override", "overrides", "overridden",
			"trigger", "triggers", "triggered",
			"need", "needs", "needed",
		},
		"irregular_participle": {
			"taken", "given", "broken", "written", "spoken", "hidden",
			"eaten", "seen", "fallen", "driven", "frozen", "beaten",
			"forgotten", "shaken", "thrown", "gone",
		},
		"participle_head_exception": {
			"feed", "seed", "need", "deed", "speed", "breed", "weed",
			"reed", "greed", "creed", "shed", "sled", "bleed", "plead",
			"indeed", "succeed", "exceed", "proceed",
		},
		"garden_path_head": {
			"prune", "build", "run", "test", "set", "cut", "filter", "sort",
			"push", "pull", "commit", "merge", "patch", "query", "load", "save",
			"copy", "delete", "insert", "update", "change", "review", "release",
			"deploy", "design", "plan", "record", "trace", "sample", "slice",
			"split", "join", "wrap", "bind", "cast", "link", "feed", "seed",
			"shed", "trust", "block", "lock", "mount", "reset", "format",
			"hash", "cache", "queue", "stack", "loop", "log", "map", "match",
			"scale", "schedule", "batch", "stage", "compile", "parse", "render",
			"watch", "monitor", "backup", "restore", "export", "import",
		},
		"determiner": {
			"the", "a", "an", "this", "that", "these", "those",
			"some", "any", "each", "every", "both", "either", "neither",
			"all", "few", "many", "much", "more", "most", "less", "least",
			"no", "another", "other", "same", "such", "what", "which", "whose",
			"my", "your", "his", "her", "its", "our", "their",
		},
		"gerund_determiner": {
			"the", "a", "an", "this", "that", "your", "our", "their",
			"his", "her", "its", "some", "any", "every", "each",
			"all", "no", "these", "those",
		},
		"known_abbrev": {
			"api", "url", "uri", "uuid", "guid", "http", "https", "html",
			"css", "js", "ts", "tsx", "jsx", "json", "xml", "csv", "yaml",
			"yml", "toml", "sql", "cli", "sdk", "ui", "ux", "id", "ids",
		},
	}
}

func TestSentenceLength(t *testing.T) {
	ctx := &RunContext{Document: testDoc("one two three four five six."), Profile: testProfile()}
	findings, err := Get("CORE.SENTENCE_LENGTH").Run(ctx)
	if err != nil || len(findings) == 0 {
		t.Fatal("expected sentence length finding")
	}
}

func TestSentenceLengthCodeCommentUsesDescriptionLimit(t *testing.T) {
	doc := testDoc("one two three four five six.")
	doc.Kind = "code-comment"
	ctx := &RunContext{Document: doc, Profile: testProfile()}
	findings, err := Get("CORE.SENTENCE_LENGTH").Run(ctx)
	if err != nil || len(findings) == 0 {
		t.Fatal("expected code comment to inherit description sentence limit")
	}
}

func TestTermDiscouraged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("deprecated term appears here."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected discouraged term finding")
	}
}
func TestTermDiscouragedHonorsRulePolicy(t *testing.T) {
	p := testProfile()
	for i := range p.Rules.Rules {
		if p.Rules.Rules[i].ID == "CORE.TERM_DISCOURAGED" {
			p.Rules.Rules[i].Enforcement = "candidate"
			p.Rules.Rules[i].Severity = "info"
		}
	}
	ctx := &RunContext{Document: testDoc("deprecated term appears here."), Profile: p}
	findings, err := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	if err != nil || len(findings) != 1 {
		t.Fatalf("expected one discouraged term finding, got %d, err=%v", len(findings), err)
	}
	if findings[0].Enforcement != "candidate" || findings[0].Severity != "info" {
		t.Fatalf("expected candidate/info policy, got %s/%s", findings[0].Enforcement, findings[0].Severity)
	}
}

func TestTermDiscouragedSupportsGuidanceOnlyEntry(t *testing.T) {
	p := testProfile()
	p.Dict.Entries[1].Alternatives = nil
	ctx := &RunContext{Document: testDoc("deprecated term appears here."), Profile: p}
	findings, err := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	if err != nil || len(findings) != 1 {
		t.Fatalf("expected one discouraged term finding, got %d, err=%v", len(findings), err)
	}
	if findings[0].Suggestion != nil {
		t.Fatalf("expected no literal suggestion, got %q", *findings[0].Suggestion)
	}
	if findings[0].Evidence != "'deprecated term' is discouraged" {
		t.Fatalf("unexpected guidance-only evidence: %q", findings[0].Evidence)
	}
}

func TestTermDiscouragedMultipleOccurrences(t *testing.T) {
	ctx := &RunContext{Document: testDoc("deprecated term and deprecated term again."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	if len(findings) < 2 {
		t.Fatalf("expected at least 2 findings for multiple occurrences, got %d", len(findings))
	}
}

func TestTermDiscouragedLongestPhraseWins(t *testing.T) {
	ctx := &RunContext{Document: testDoc("deprecated term longer phrase is here."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected discouraged term finding")
	}
	// The longest matching phrase should be found ("deprecated term longer phrase"), not the shorter one
	if len(findings) > 1 {
		t.Fatalf("expected exactly 1 finding for longest-phrase match, got %d", len(findings))
	}
	expectedEvidence := "'deprecated term longer phrase' is discouraged; use 'better phrase' instead"
	if findings[0].Evidence != expectedEvidence {
		t.Fatalf("expected evidence %q, got %q", expectedEvidence, findings[0].Evidence)
	}
}

func TestTermDiscouragedNoMidWordMatch(t *testing.T) {
	ctx := &RunContext{Document: testDoc("deprecated terminology is bad."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	for _, f := range findings {
		if strings.Contains(f.Evidence, "deprecated term ") && !strings.Contains(f.Evidence, "deprecated term longer") {
			t.Fatalf("should not match 'deprecated term' inside 'deprecated terminology': %s", f.Evidence)
		}
	}
}

func TestTermDiscouragedUnicode(t *testing.T) {
	canonical := "WriteTighter"
	dict := &profile.Dictionary{FormatVersion: 1, Entries: []profile.Entry{
		{Term: "café", Status: profile.StatusDiscouraged, Alternatives: []string{"coffee shop"}, Reason: "use coffee shop", PartsOfSpeech: []string{"noun"}},
		{Term: "deprecated term", Status: profile.StatusDiscouraged, Alternatives: []string{"preferred term"}, Reason: "use preferred term", PartsOfSpeech: []string{"noun"}},
		{Term: "WriteTighter", Status: profile.StatusPreferred, PartsOfSpeech: []string{"proper noun"}, CanonicalCase: &canonical},
	}}
	_ = dict.Validate()
	rules := []profile.Rule{
		{ID: "CORE.SENTENCE_LENGTH", Enabled: true, Parameters: map[string]any{"description_max_words": 5}},
		{ID: "CORE.DENSE_PARAGRAPH", Enabled: true},
		{ID: "CORE.TERM_DISCOURAGED", Enabled: true},
		{ID: "CORE.TERM_CASE", Enabled: true},
		{ID: "CORE.TERM_UNKNOWN", Enabled: true},
		{ID: "CORE.TERM_CONSISTENCY", Enabled: true},
		{ID: "CORE.PROCEDURE_MULTI_ACTION", Enabled: true},
		{ID: "CORE.NOUN_STACK", Enabled: true, Parameters: map[string]any{"min_stack_length": 3}},
		{ID: "CORE.GERUND_OPENER", Enabled: true},
		{ID: "CORE.CONTRACTION", Enabled: true},
		{ID: "CORE.BANNED_MODAL", Enabled: true},
		{ID: "CORE.LATIN_ABBREV", Enabled: true},
		{ID: "CORE.UNEXPANDED_ABBREV", Enabled: true},
	}
	p := &profile.Resolution{Rules: &profile.RulesConfig{UnknownTermPolicy: "candidate", Rules: rules}, Dict: dict}
	ctx := &RunContext{Document: testDoc("café culture matters."), Profile: p}
	findings, _ := Get("CORE.TERM_DISCOURAGED").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected discouraged term finding for Unicode term")
	}
}
func TestDenseParagraph(t *testing.T) {
	ctx := &RunContext{Document: testDoc("One. Two. Three. Four."), Profile: testProfile()}
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
	// Basic unknown term
	ctx := &RunContext{Document: testDoc("flarb is unknown."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_UNKNOWN").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected unknown term finding")
	}
}

func TestTermUnknownContraction(t *testing.T) {
	// "don't" should be treated as a single token, not split into "don" and "t"
	ctx := &RunContext{Document: testDoc("you don't know flarb."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_UNKNOWN").Run(ctx)
	// Count how many findings mention parts of the contraction
	donCount := 0
	tCount := 0
	dontCount := 0
	for _, f := range findings {
		e := f.Evidence
		if e == "Unknown term: don" {
			donCount++
		}
		if e == "Unknown term: t" {
			tCount++
		}
		if strings.Contains(e, "don'") && !strings.Contains(e, "flarb") {
			dontCount++
		}
	}
	if donCount > 0 {
		t.Fatalf("contraction incorrectly split: found standalone 'don' finding")
	}
	if tCount > 0 {
		t.Fatalf("contraction incorrectly split: found standalone 't' finding")
	}
	// "don't" should appear as a single unknown token
	if dontCount == 0 {
		t.Fatalf("expected contraction to be found as a single unknown token, got: %v", findings)
	}
	// Should also find "flarb" as unknown
	foundFlarb := false
	for _, f := range findings {
		if strings.Contains(f.Evidence, "flarb") {
			foundFlarb = true
		}
	}
	if !foundFlarb {
		t.Fatal("expected 'flarb' to be found as unknown")
	}
}

func TestTermUnknownHyphenatedKnown(t *testing.T) {
	// "check-in" is in the dictionary with StatusPreferred, so it should NOT produce a finding
	ctx := &RunContext{Document: testDoc("do a check-in now."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_UNKNOWN").Run(ctx)
	for _, f := range findings {
		if strings.Contains(f.Evidence, "check-in") {
			t.Fatalf("hyphenated known term 'check-in' should not be unknown: %s", f.Evidence)
		}
	}
}

func TestTermUnknownMultiWordPhrase(t *testing.T) {
	// "deprecated term" is in the dictionary (StatusDiscouraged), so it should NOT be unknown
	ctx := &RunContext{Document: testDoc("this deprecated term appears here."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_UNKNOWN").Run(ctx)
	for _, f := range findings {
		if strings.Contains(f.Evidence, "deprecated") && strings.Contains(f.Evidence, "term") {
			t.Fatalf("multi-word phrase 'deprecated term' should match dictionary: %s", f.Evidence)
		}
	}
}

func TestTermUnknownUnicode(t *testing.T) {
	// Unicode terms should work correctly
	ctx := &RunContext{Document: testDoc("über is a German word."), Profile: testProfile()}
	findings, _ := Get("CORE.TERM_UNKNOWN").Run(ctx)
	// "über" should be found as unknown (not in dict)
	foundUber := false
	for _, f := range findings {
		if strings.Contains(f.Evidence, "über") {
			foundUber = true
		}
	}
	if !foundUber {
		t.Fatal("expected 'über' to be found as unknown")
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

func TestInsensitiveMatchesUnicode(t *testing.T) {
	// Test that insensitiveMatches finds terms correctly in Unicode text.
	// "über" should match "ÜBER" case-insensitively.
	matches := insensitiveMatches("ÜBER cool", "über")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	// Test multi-byte UTF-8 match offsets are correct
	matches = insensitiveMatches("über cool", "über")
	// "über" is 5 bytes in UTF-8 (ü = 0xC3 0xBC)
	if len(matches) != 1 || matches[0][0] != 0 || matches[0][1] != 5 {
		t.Fatalf("expected match at [0,5), got %v", matches)
	}
	// Test word boundary: "übercool" should NOT match "über"
	matches = insensitiveMatches("übercool", "über")
	if len(matches) != 0 {
		t.Fatalf("expected no mid-word match, got %d", len(matches))
	}
	// Word boundary rules: CJK all-letter chars mean the term must be bounded by
	// non-letter or string start/end on both sides. "世界" in "A世界B" would match
	// because 'A' and 'B' are letters (no match). With a real boundary like space,
	// " 世界 " would match. But this is English-first, so CJK word boundaries use
	// the same letter/digit detection.
	// At minimum, a single CJK term in isolation should match:
	matches = insensitiveMatches("世界", "世界")
	if len(matches) != 1 {
		t.Fatalf("expected 1 CJK exact match, got %d", len(matches))
	}
}

func TestCodePointColumn(t *testing.T) {
	// "über" has byte offsets 0,1,2,3,4 but rune offsets 0,1,2,3
	// codePointColumn should return proper column counting runes.
	col := codePointColumn("über text", 0, 1) // byte 0 = rune index 0, start at col 1
	if col != 1 {
		t.Fatalf("expected col 1 for byte 0, got %d", col)
	}
	col = codePointColumn("über text", 4, 1) // byte 4 = rune index 3
	if col != 4 {
		t.Fatalf("expected col 4 for byte 4 (after 'ü'+'b'+'e'+'r'), got %d", col)
	}
}

func TestNounStack(t *testing.T) {
	ctx := &RunContext{Document: testDoc("The assumptions-side entry wrappers are configured."), Profile: testProfile()}
	findings, _ := Get("CORE.NOUN_STACK").Run(ctx)
	if len(findings) == 0 {
		t.Fatal("expected noun stack finding for 'assumptions-side entry wrappers'")
	}
	if !strings.Contains(findings[0].Evidence, "assumptions-side") {
		t.Fatalf("expected evidence to contain the noun stack, got %q", findings[0].Evidence)
	}
}

func TestNounStackShortRunNotFlagged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("The hot loop is fast."), Profile: testProfile()}
	findings, _ := Get("CORE.NOUN_STACK").Run(ctx)
	for _, f := range findings {
		if strings.Contains(f.Evidence, "hot loop") {
			t.Fatalf("2-word run should not be flagged: %s", f.Evidence)
		}
	}
}

func TestNounStackThreshold(t *testing.T) {
	p := testProfile()
	for i := range p.Rules.Rules {
		if p.Rules.Rules[i].ID == "CORE.NOUN_STACK" {
			p.Rules.Rules[i].Parameters = map[string]any{"min_stack_length": 4}
		}
	}
	ctx := &RunContext{Document: testDoc("The assumptions-side entry wrappers are configured."), Profile: p}
	findings, _ := Get("CORE.NOUN_STACK").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("3-word stack should not be flagged at threshold 4, got %d findings", len(findings))
	}
}

func TestGerundOpener(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Arming the assumptions-side entry wrappers and consume in their recursive cores."), Profile: testProfile()}
	findings, _ := Get("CORE.GERUND_OPENER").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 gerund opener finding, got %d", len(findings))
	}
	if !strings.Contains(findings[0].Evidence, "Arming") {
		t.Fatalf("expected evidence to contain 'Arming', got %q", findings[0].Evidence)
	}
}

func TestGerundOpenerNotInList(t *testing.T) {
	ctx := &RunContext{Document: testDoc("1. Configuring the server requires a restart.\n2. Running the test suite.")}
	findings, _ := Get("CORE.GERUND_OPENER").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("expected no findings in list items, got %d: %v", len(findings), findings)
	}
}

func TestGerundOpenerShortWordNotFlagged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Ring the bell to signal completion."), Profile: testProfile()}
	findings, _ := Get("CORE.GERUND_OPENER").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("'Ring' (4 runes) should not be flagged, got %d: %v", len(findings), findings)
	}
}

func TestGerundOpenerNoDeterminerNotFlagged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Running takes five minutes."), Profile: testProfile()}
	findings, _ := Get("CORE.GERUND_OPENER").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("gerund without determiner should not be flagged, got %d: %v", len(findings), findings)
	}
}

func TestGerundOpenerBaseVerbNotFlagged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Bring the server online."), Profile: testProfile()}
	findings, _ := Get("CORE.GERUND_OPENER").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("'Bring' is a base verb (stem 'br' < 3), should not be flagged, got %d: %v", len(findings), findings)
	}
}

func TestGerundOpenerSixCharFlagged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Arming the system takes time."), Profile: testProfile()}
	findings, _ := Get("CORE.GERUND_OPENER").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("'Arming' (6 runes, stem 'arm' = 3) should be flagged, got %d: %v", len(findings), findings)
	}
}

func TestContraction(t *testing.T) {
	ctx := &RunContext{Document: testDoc("You'll need to don't worry, it's fine."), Profile: testProfile()}
	findings, _ := Get("CORE.CONTRACTION").Run(ctx)
	if len(findings) != 3 {
		t.Fatalf("expected exactly 3 contraction findings (You'll, don't, it's), got %d: %v", len(findings), findings)
	}
}

func TestContractionSuffixes(t *testing.T) {
	ctx := &RunContext{Document: testDoc("We've tried. They're here. I'd say. He'll know."), Profile: testProfile()}
	findings, _ := Get("CORE.CONTRACTION").Run(ctx)
	if len(findings) != 4 {
		t.Fatalf("expected 4 suffix contractions ('ve, 're, 'd, 'll), got %d: %v", len(findings), findings)
	}
}

func TestContractionNotInCodeSpan(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Run `git push --force` if you're sure."), Profile: testProfile()}
	findings, _ := Get("CORE.CONTRACTION").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 contraction finding (you're), got %d: %v", len(findings), findings)
	}
}

func TestContractionPossessiveNotFlagged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("The server's configuration file is correct."), Profile: testProfile()}
	findings, _ := Get("CORE.CONTRACTION").Run(ctx)
	for _, f := range findings {
		if strings.Contains(f.Evidence, "server's") {
			t.Fatalf("possessive 'server's' should not be flagged as contraction: %s", f.Evidence)
		}
	}
}

func TestContractionNilContext(t *testing.T) {
	checker := Get("CORE.CONTRACTION")
	findings, err := checker.Run(nil)
	if err != nil || findings != nil {
		t.Fatalf("nil context should return empty, got %d findings, err=%v", len(findings), err)
	}
	findings, err = checker.Run(&RunContext{Document: nil})
	if err != nil || findings != nil {
		t.Fatalf("nil document should return empty, got %d findings, err=%v", len(findings), err)
	}
}

func TestContractionHonorsRulePolicy(t *testing.T) {
	p := testProfile()
	for i := range p.Rules.Rules {
		if p.Rules.Rules[i].ID == "CORE.CONTRACTION" {
			p.Rules.Rules[i].Enforcement = "enforced"
			p.Rules.Rules[i].Severity = "warning"
		}
	}
	ctx := &RunContext{Document: testDoc("It's fine."), Profile: p}
	findings, _ := Get("CORE.CONTRACTION").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Enforcement != "enforced" || findings[0].Severity != "warning" {
		t.Fatalf("expected enforced/warning, got %s/%s", findings[0].Enforcement, findings[0].Severity)
	}
}

func TestBannedModal(t *testing.T) {
	ctx := &RunContext{Document: testDoc("You should verify the config. It may fail."), Profile: testProfile()}
	findings, _ := Get("CORE.BANNED_MODAL").Run(ctx)
	if len(findings) != 2 {
		t.Fatalf("expected 2 banned modal findings (should, may), got %d: %v", len(findings), findings)
	}
}

func TestBannedModalAllVariants(t *testing.T) {
	ctx := &RunContext{Document: testDoc("You should, would, may, might, could try."), Profile: testProfile()}
	findings, _ := Get("CORE.BANNED_MODAL").Run(ctx)
	if len(findings) != 5 {
		t.Fatalf("expected 5 banned modal findings (should, would, may, might, could), got %d: %v", len(findings), findings)
	}
}

func TestBannedModalNotInCodeSpan(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Run `should` in a test. You must verify."), Profile: testProfile()}
	findings, _ := Get("CORE.BANNED_MODAL").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("'should' inside code span should not be flagged, got %d: %v", len(findings), findings)
	}
}

func TestBannedModalApprovedNotFlagged(t *testing.T) {
	ctx := &RunContext{Document: testDoc("You can and must verify the config."), Profile: testProfile()}
	findings, _ := Get("CORE.BANNED_MODAL").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("approved modals (can, must) should not be flagged, got %d: %v", len(findings), findings)
	}
}

func TestBannedModalNilContext(t *testing.T) {
	checker := Get("CORE.BANNED_MODAL")
	findings, err := checker.Run(nil)
	if err != nil || findings != nil {
		t.Fatalf("nil context should return empty, got %d findings, err=%v", len(findings), err)
	}
	findings, err = checker.Run(&RunContext{Document: nil})
	if err != nil || findings != nil {
		t.Fatalf("nil document should return empty, got %d findings, err=%v", len(findings), err)
	}
}

func TestBannedModalHonorsRulePolicy(t *testing.T) {
	p := testProfile()
	for i := range p.Rules.Rules {
		if p.Rules.Rules[i].ID == "CORE.BANNED_MODAL" {
			p.Rules.Rules[i].Enforcement = "enforced"
			p.Rules.Rules[i].Severity = "warning"
		}
	}
	ctx := &RunContext{Document: testDoc("You should try."), Profile: p}
	findings, _ := Get("CORE.BANNED_MODAL").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Enforcement != "enforced" || findings[0].Severity != "warning" {
		t.Fatalf("expected enforced/warning, got %s/%s", findings[0].Enforcement, findings[0].Severity)
	}
}

func TestBannedModalCustomSuggestion(t *testing.T) {
	ctx := &RunContext{Document: testDoc("You should verify."), Profile: testProfile()}
	findings, _ := Get("CORE.BANNED_MODAL").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if !strings.Contains(findings[0].Message, "Custom:") {
		t.Fatalf("expected custom dictionary suggestion in message, got %q", findings[0].Message)
	}
}

func TestLatinAbbrev(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Use the flag, e.g. --force. See the docs (i.e. README). And more etc."), Profile: testProfile()}
	findings, _ := Get("CORE.LATIN_ABBREV").Run(ctx)
	if len(findings) != 3 {
		t.Fatalf("expected exactly 3 Latin abbreviation findings (e.g., i.e., etc.), got %d: %v", len(findings), findings)
	}
}

func TestLatinAbbrevTrailingPeriod(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Use files, logs, etc. for debugging."), Profile: testProfile()}
	findings, _ := Get("CORE.LATIN_ABBREV").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for 'etc.' with trailing period, got %d: %v", len(findings), findings)
	}
}

func TestLatinAbbrevNotInCodeSpan(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Run `pip install -e .` to install."), Profile: testProfile()}
	findings, _ := Get("CORE.LATIN_ABBREV").Run(ctx)
	for _, f := range findings {
		if strings.Contains(f.Evidence, "-e") {
			t.Fatalf("'e.' inside code span should not be flagged: %s", f.Evidence)
		}
	}
}

func TestLatinAbbrevNilContext(t *testing.T) {
	checker := Get("CORE.LATIN_ABBREV")
	findings, err := checker.Run(nil)
	if err != nil || findings != nil {
		t.Fatalf("nil context should return empty, got %d findings, err=%v", len(findings), err)
	}
	findings, err = checker.Run(&RunContext{Document: nil})
	if err != nil || findings != nil {
		t.Fatalf("nil document should return empty, got %d findings, err=%v", len(findings), err)
	}
}

func TestLatinAbbrevHonorsRulePolicy(t *testing.T) {
	p := testProfile()
	for i := range p.Rules.Rules {
		if p.Rules.Rules[i].ID == "CORE.LATIN_ABBREV" {
			p.Rules.Rules[i].Enforcement = "enforced"
			p.Rules.Rules[i].Severity = "warning"
		}
	}
	ctx := &RunContext{Document: testDoc("Use e.g. this."), Profile: p}
	findings, _ := Get("CORE.LATIN_ABBREV").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Enforcement != "enforced" || findings[0].Severity != "warning" {
		t.Fatalf("expected enforced/warning, got %s/%s", findings[0].Enforcement, findings[0].Severity)
	}
}

func TestLatinAbbrevCustomSuggestion(t *testing.T) {
	ctx := &RunContext{Document: testDoc("Use e.g. this."), Profile: testProfile()}
	findings, _ := Get("CORE.LATIN_ABBREV").Run(ctx)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if !strings.Contains(findings[0].Message, "Custom:") {
		t.Fatalf("expected custom dictionary suggestion in message, got %q", findings[0].Message)
	}
}

func TestNounStackSkipsHeadings(t *testing.T) {
	ctx := &RunContext{Document: testDoc("## Review fixes applied\n\nSome prose here.")}
	findings, _ := Get("CORE.NOUN_STACK").Run(ctx)
	for _, f := range findings {
		if strings.Contains(f.Evidence, "Review fixes applied") {
			t.Fatalf("noun stack should not flag heading: %s", f.Evidence)
		}
	}
}

func TestNounStackSkipsListItems(t *testing.T) {
	ctx := &RunContext{Document: testDoc("- method returning OS process ID\n- fix early return guard")}
	findings, _ := Get("CORE.NOUN_STACK").Run(ctx)
	if len(findings) != 0 {
		t.Fatalf("noun stack should not flag list items, got %d: %v", len(findings), findings)
	}
}
