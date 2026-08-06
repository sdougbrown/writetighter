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
	dict := &profile.Dictionary{FormatVersion: 1}
	_ = dict.Validate()
	return &profile.Resolution{Rules: &profile.RulesConfig{Rules: rules}, Dict: dict}
}

// corpusNoveltyProfileWithDict returns a profile with a dictionary entry for "tagline".
func corpusNoveltyProfileWithDict() *profile.Resolution {
	rules := []profile.Rule{
		{ID: "CORE.CORPUS_NOVELTY", Enabled: true, Enforcement: "candidate", Severity: "info"},
	}
	e := profile.Entry{Term: "tagline", Status: profile.StatusAllowed, PartsOfSpeech: []string{"noun"}}
	dict := &profile.Dictionary{FormatVersion: 1, Entries: []profile.Entry{e}}
	_ = dict.Validate()
	return &profile.Resolution{Rules: &profile.RulesConfig{Rules: rules}, Dict: dict}
}

func corpusNoveltyDoc(text string) *document.Document {
	doc, _ := document.FromReader(strings.NewReader(text), "test.md", "description")
	return doc
}

func TestCorpusNoveltyAbstainsWithoutBaseline(t *testing.T) {
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The fluxion capacitor drives the warp core."),
		Profile:  corpusNoveltyProfile(),
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings without baseline, got %d", len(findings))
	}
}

func TestCorpusNoveltyFlagsNovelTermWithRepetition(t *testing.T) {
	baseline := &corpus.Baseline{
		Revision:    "abc12345",
		TermCounts:  map[string]int{"overgrid": 0},
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
		Document: corpusNoveltyDoc("The bracket-mesh overgrid provides the factory keys for building test overrides. The bracket-mesh overgrid re-derives item metadata."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
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
		if !strings.Contains(f.Evidence, "baseline_count=0") {
			t.Errorf("expected baseline_count=0 in evidence: %s", f.Evidence)
		}
		if !strings.Contains(f.Evidence, "abc12345") {
			t.Errorf("expected baseline revision in evidence: %s", f.Evidence)
		}
	}
}

func TestCorpusNoveltyDoesNotFlagEstablishedTerm(t *testing.T) {
	baseline := &corpus.Baseline{
		Revision:    "abc12345",
		TermCounts:  map[string]int{"tagline": 3}, // established in baseline
		PhraseCounts: map[string]int{},
		ChangeTermCounts: map[string]int{
			"tagline": 2,
		},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The tagline sorter reads the tagline from the camera feed."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
	}
	findings, err := Get("CORE.CORPUS_NOVELTY").Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.Evidence, "tagline") {
			t.Errorf("should not flag 'tagline' which has baseline precedent: %s", f.Evidence)
		}
	}
}

func TestCorpusNoveltyExcludesIdentifiers(t *testing.T) {
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{"bracketindexid": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("bracketIndexId holds the raw numeric registryId value."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
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
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{"provides": 3, "factory": 2, "keys": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("provides factory keys provides factory keys provides factory keys."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
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
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{"tagline": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The tagline sorter reads the tagline from the camera feed."),
		Profile:  corpusNoveltyProfileWithDict(),
		Baseline: baseline,
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
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{"warmer": 2},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The warmer rehydrates cached query data. The warmer invalidates the cache."),
		Profile:  corpusNoveltyProfileWithDict(),
		Terms:    terms,
		Baseline: baseline,
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
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{"nightmap": 1}, // only 1 occurrence
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The nightmap parameter controls rendering."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
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
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("See https://example.com/docs for details. See https://example.com/docs again."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
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
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{"overgrid": 3},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The overgrid provides the factory keys. The overgrid re-derives metadata. The overgrid builds overrides."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
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
	baseline := &corpus.Baseline{
		Revision:         "abc12345",
		TermCounts:       map[string]int{},
		PhraseCounts:     map[string]int{},
		ChangeTermCounts: map[string]int{"overgrid": 5},
		ChangePhraseCounts: map[string]int{},
	}
	ctx := &RunContext{
		Document: corpusNoveltyDoc("The overgrid provides keys. The overgrid re-derives metadata. The overgrid builds overrides."),
		Profile:  corpusNoveltyProfile(),
		Baseline: baseline,
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