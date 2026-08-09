package check

import (
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/corpus"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
)

// corpusNoveltyProfile returns a profile with CORE.CORPUS_NOVELTY enabled.
func corpusNoveltyProfile() *profile.Resolution {
	rules := []profile.Rule{
		{ID: "CORE.CORPUS_NOVELTY", Enabled: true, Enforcement: "candidate", Severity: "info"},
	}
	wc := testWordClasses()
	// Ensure the specific stopwords tested by TestCorpusNoveltyExcludesStopwords
	// are present in the corpus_novelty_stopword class.
	wc["corpus_novelty_stopword"] = append([]string{}, wc["stopword"]...)
	extra := []string{"provides", "factory", "keys", "new", "null", "out",
		"building", "partial", "test", "provides", "factory", "keys"}
	for _, w := range extra {
		if !contains(wc["corpus_novelty_stopword"], w) {
			wc["corpus_novelty_stopword"] = append(wc["corpus_novelty_stopword"], w)
		}
	}
	dict := &profile.Dictionary{FormatVersion: 2, WordClasses: wc}
	_ = dict.Validate()
	return &profile.Resolution{Rules: &profile.RulesConfig{Rules: rules}, Dict: dict}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// corpusNoveltyProfileWithDict returns a profile with a dictionary entry for "tagline".
func corpusNoveltyProfileWithDict() *profile.Resolution {
	rules := []profile.Rule{
		{ID: "CORE.CORPUS_NOVELTY", Enabled: true, Enforcement: "candidate", Severity: "info"},
	}
	e := profile.Entry{Term: "tagline", Status: profile.StatusAllowed, PartsOfSpeech: []string{"noun"}}
	wc := testWordClasses()
	wc["corpus_novelty_stopword"] = append([]string{}, wc["stopword"]...)
	dict := &profile.Dictionary{FormatVersion: 2, Entries: []profile.Entry{e}, WordClasses: wc}
	_ = dict.Validate()
	return &profile.Resolution{Rules: &profile.RulesConfig{Rules: rules}, Dict: dict}
}

func corpusNoveltyDoc(text string) *document.Document {
	doc, _ := document.FromReader(strings.NewReader(text), "test.md", "description")
	return doc
}

func TestCorpusNoveltyAbstainsWithoutGitCompare(t *testing.T) {
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The fluxion capacitor drives the warp core."),
		Profile:  corpusNoveltyProfile(),
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings without gitCompare, got %d", len(findings))
	}
}

func TestCorpusNoveltyFlagsNovelTermWithRepetition(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:     "abc12345",
		TermCounts:   map[string]int{"overgrid": 0},
		PhraseCounts: map[string]int{},
		ChangeTermCounts: map[string]int{
			"overgrid":     4,
			"bracket-mesh": 4,
		},
		ChangePhraseCounts: map[string]int{
			"bracket-mesh overgrid": 4,
		},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The bracket-mesh overgrid provides the factory keys for building test overrides. The bracket-mesh overgrid re-derives item metadata."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for novel terms 'overgrid' and 'bracket-mesh'")
	}
	// Check that "overgrid" and "bracket-mesh" are both flagged
	foundOvergrid := false
	foundBracketMesh := false
	foundPhrase := false
	for _, f := range findings {
		if strings.Contains(f.Evidence, "overgrid") && !strings.Contains(f.Evidence, "bracket-mesh overgrid") {
			foundOvergrid = true
		}
		if strings.Contains(f.Evidence, "bracket-mesh") && !strings.Contains(f.Evidence, "bracket-mesh overgrid") {
			foundBracketMesh = true
		}
		if strings.Contains(f.Evidence, "bracket-mesh overgrid") {
			foundPhrase = true
		}
	}
	if !foundOvergrid {
		t.Error("expected 'overgrid' token finding")
	}
	if !foundBracketMesh {
		t.Error("expected 'bracket-mesh' token finding")
	}
	if !foundPhrase {
		t.Error("expected 'bracket-mesh overgrid' phrase finding")
	}
	// Verify provenance
	for _, f := range findings {
		if !strings.Contains(f.Evidence, "git_compare_count=0") {
			t.Errorf("expected git_compare_count=0 in evidence: %s", f.Evidence)
		}
		if !strings.Contains(f.Evidence, "abc12345") {
			t.Errorf("expected gitCompare revision in evidence: %s", f.Evidence)
		}
	}
}

func TestCorpusNoveltyDoesNotFlagEstablishedTerm(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:     "abc12345",
		TermCounts:   map[string]int{"tagline": 3}, // established in gitCompare
		PhraseCounts: map[string]int{},
		ChangeTermCounts: map[string]int{
			"tagline": 2,
		},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The tagline sorter reads the tagline from the camera feed."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.Evidence, "tagline") {
			t.Errorf("should not flag 'tagline' which has gitCompare precedent: %s", f.Evidence)
		}
	}
}

func TestCorpusNoveltyExcludesIdentifiers(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"bracketindexid": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("bracketIndexId holds the raw numeric registryId value."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (identifiers excluded), got %d: %v", len(findings), findings)
	}
}

func TestCorpusNoveltyExcludesStopwords(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"provides": 3, "factory": 2, "keys": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("provides factory keys provides factory keys provides factory keys."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (stopwords excluded), got %d: %v", len(findings), findings)
	}
}

func TestCorpusNoveltyExcludesProfileDictionaryTerms(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"tagline": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The tagline sorter reads the tagline from the camera feed."),
		Profile:    corpusNoveltyProfileWithDict(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.Evidence, "tagline") {
			t.Errorf("should not flag 'tagline' which is in profile dictionary: %s", f.Evidence)
		}
	}
}

func TestCorpusNoveltyExcludesProjectTerms(t *testing.T) {
	terms := []config.TermEntry{
		{Term: "warmer", PartsOfSpeech: []string{"noun"}, Definition: "Rehydration component."},
	}
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"warmer": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The warmer rehydrates cached query data. The warmer invalidates the cache."),
		Profile:    corpusNoveltyProfileWithDict(),
		Terms:      terms,
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.Evidence, "warmer") {
			t.Errorf("should not flag 'warmer' which is a project term: %s", f.Evidence)
		}
	}
}

func TestCorpusNoveltyRespectsMinRepetition(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"nightmap": 1}, // only 1 occurrence
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The nightmap parameter controls rendering."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (change_count=1 < min_repetition=2), got %d", len(findings))
	}
}

func TestCorpusNoveltyExcludesURLsAndPaths(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("See https://example.com/docs for details. See https://example.com/docs again."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (URLs excluded), got %d: %v", len(findings), findings)
	}
}

func TestCorpusNoveltyIsAdvisoryOnly(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"overgrid": 3},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The overgrid provides the factory keys. The overgrid re-derives metadata. The overgrid builds overrides."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}
	for _, f := range findings {
		if f.Enforcement != "candidate" {
			t.Errorf("expected enforcement=candidate, got %s", f.Enforcement)
		}
		if f.Severity != "info" {
			t.Errorf("expected severity=info, got %s", f.Severity)
		}
		// Message should ask for clarification, not declare invalid
		if !strings.Contains(f.Message, "consider") {
			t.Errorf("message should be advisory: %s", f.Message)
		}
	}
}

func TestCorpusNoveltyDeduplicatesPerDocument(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"overgrid": 5},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The overgrid provides keys. The overgrid re-derives metadata. The overgrid builds overrides."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Should report "overgrid" only once per document
	overgridCount := 0
	for _, f := range findings {
		if strings.Contains(f.Evidence, "overgrid") && !strings.Contains(f.Evidence, "bracket-mesh") {
			overgridCount++
		}
	}
	if overgridCount != 1 {
		t.Errorf("expected 1 finding for 'overgrid', got %d", overgridCount)
	}
}

func TestCorpusNoveltyMinRepetitionOneFlagsSingleOccurrence(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "abc12345",
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"nightmap": 1}, // single occurrence
		ChangePhraseCounts: map[string]int{},
	}
	rules := []profile.Rule{
		{ID: "CORE.CORPUS_NOVELTY", Enabled: true, Enforcement: "candidate", Severity: "info", Parameters: map[string]interface{}{"min_repetition": 1}},
	}
	dict := &profile.Dictionary{FormatVersion: 1}
	_ = dict.Validate()
	profile := &profile.Resolution{Rules: &profile.RulesConfig{Rules: rules}, Dict: dict}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The nightmap parameter controls rendering."),
		Profile:    profile,
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected finding for 'nightmap' with min_repetition=1")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Evidence, "nightmap") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'nightmap' finding with min_repetition=1")
	}
}

func TestCorpusNoveltyShortGitCompareRevisionSafe(t *testing.T) {
	gitCompare := &corpus.GitCompare{
		Revision:           "ab", // shorter than 8 chars
		TermCounts:         map[string]int{},
		PhraseCounts:       map[string]int{},
		ChangeTermCounts:   map[string]int{"overgrid": 3},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document:   corpusNoveltyDoc("The overgrid provides keys. The overgrid re-derives metadata. The overgrid builds overrides."),
		Profile:    corpusNoveltyProfile(),
		GitCompare: gitCompare,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for novel term")
	}
	// Verify the full revision appears in evidence (not truncated)
	for _, f := range findings {
		if !strings.Contains(f.Evidence, "ab") {
			t.Errorf("expected short revision in evidence: %s", f.Evidence)
		}
		// Message should contain the full revision, not panic on [:8]
		if !strings.Contains(f.Message, "ab") {
			t.Errorf("expected short revision in message: %s", f.Message)
		}
	}
}
