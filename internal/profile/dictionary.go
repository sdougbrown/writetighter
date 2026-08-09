package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type EntryStatus string

const (
	StatusPreferred   EntryStatus = "preferred"
	StatusAllowed     EntryStatus = "allowed"
	StatusDiscouraged EntryStatus = "discouraged"
	StatusObserved    EntryStatus = "observed"
)

type Entry struct {
	Term          string      `json:"term"`
	PartsOfSpeech []string    `json:"parts_of_speech,omitempty"`
	Status        EntryStatus `json:"status"`
	Alternatives  []string    `json:"alternatives,omitempty"`
	CanonicalCase *string     `json:"canonical_case"`
	Reason        string      `json:"reason,omitempty"`
	EvidenceRefs  []string    `json:"evidence_refs,omitempty"`
}

type Dictionary struct {
	FormatVersion                int                 `json:"format_version"`
	Entries                      []Entry             `json:"entries"`
	WordClasses                  map[string][]string `json:"word_classes,omitempty"`
	BannedModalSuggestions       map[string]string    `json:"banned_modal_suggestions,omitempty"`
	BannedLatinAbbrevSuggestions map[string]string    `json:"banned_latin_abbrev_suggestions,omitempty"`

	// Internal indices built during Validate; not serialized.
	index         map[string]*Entry
	wordClassSet  map[string]map[string]bool
}

func (d *Dictionary) Validate() error {
	var errs []error
	if d.FormatVersion != 1 && d.FormatVersion != 2 {
		errs = append(errs, fmt.Errorf("unsupported dictionary format_version %d", d.FormatVersion))
	}
	d.index = map[string]*Entry{}
	d.wordClassSet = map[string]map[string]bool{}

	// Build word class index.
	for class, words := range d.WordClasses {
		if class == "" {
			errs = append(errs, fmt.Errorf("word class with empty name"))
			continue
		}
		set := make(map[string]bool, len(words))
		for _, w := range words {
			if w == "" {
				errs = append(errs, fmt.Errorf("word class %q: empty word", class))
				continue
			}
			set[foldString(w)] = true
		}
		d.wordClassSet[class] = set
	}

	for i := range d.Entries {
		e := &d.Entries[i]
		if e.Term == "" {
			errs = append(errs, fmt.Errorf("entry %d: empty term", i))
			continue
		}
		switch e.Status {
		case StatusPreferred, StatusAllowed, StatusDiscouraged, StatusObserved:
		default:
			errs = append(errs, fmt.Errorf("entry %q: invalid status %q", e.Term, e.Status))
		}
		for _, pos := range e.PartsOfSpeech {
			if strings.TrimSpace(pos) == "" {
				errs = append(errs, fmt.Errorf("entry %q: empty part of speech", e.Term))
			}
		}
		if e.Status == StatusPreferred || e.Status == StatusAllowed || e.Status == StatusDiscouraged {
			if len(e.PartsOfSpeech) == 0 {
				errs = append(errs, fmt.Errorf("entry %q: status %s requires parts_of_speech", e.Term, e.Status))
			}
		}
		// Rejection rule 6: discouraged requires reason
		if e.Status == StatusDiscouraged && strings.TrimSpace(e.Reason) == "" {
			errs = append(errs, fmt.Errorf("entry %q: discouraged requires reason", e.Term))
		}
		// Rejection rule 3: duplicate entry after Unicode case folding
		k := foldString(e.Term)
		if _, ok := d.index[k]; ok {
			errs = append(errs, fmt.Errorf("duplicate term %q", e.Term))
		}
		d.index[k] = e
	}
	return errors.Join(errs...)
}

// ValidateAlternatives checks that discouraged entries with alternatives
// resolve to preferred or allowed entries in the dictionary.
func (d *Dictionary) ValidateAlternatives() error {
	if d == nil || d.index == nil {
		return nil
	}
	var errs []error
	for i := range d.Entries {
		e := &d.Entries[i]
		if e.Status != StatusDiscouraged || len(e.Alternatives) == 0 {
			continue
		}
		for j, alt := range e.Alternatives {
			resolved := d.Lookup(alt)
			if resolved == nil {
				errs = append(errs, fmt.Errorf("entry %q alternative[%d] %q does not resolve to any dictionary entry", e.Term, j, alt))
			} else if resolved.Status != StatusPreferred && resolved.Status != StatusAllowed {
				errs = append(errs, fmt.Errorf("entry %q alternative[%d] %q resolves to entry with status %q (expected preferred or allowed)", e.Term, j, alt, resolved.Status))
			}
		}
	}
	return errors.Join(errs...)
}

func (d *Dictionary) Lookup(term string) *Entry {
	if d == nil {
		return nil
	}
	return d.index[foldString(term)]
}

// HasWordClass reports whether the given word belongs to the named word class.
// The check is case-insensitive. Returns false if the word class does not exist.
func (d *Dictionary) HasWordClass(word, class string) bool {
	if d == nil || d.wordClassSet == nil {
		return false
	}
	set, ok := d.wordClassSet[class]
	if !ok {
		return false
	}
	return set[foldString(word)]
}

// WordClassSet returns the set of words in the given class, or nil if the class
// does not exist. The caller MUST NOT modify the returned map.
func (d *Dictionary) WordClassSet(class string) map[string]bool {
	if d == nil || d.wordClassSet == nil {
		return nil
	}
	return d.wordClassSet[class]
}

// HasWordClassAny reports whether the word belongs to any of the named classes.
func (d *Dictionary) HasWordClassAny(word string, classes ...string) bool {
	if d == nil || d.wordClassSet == nil {
		return false
	}
	lower := foldString(word)
	for _, class := range classes {
		if set, ok := d.wordClassSet[class]; ok && set[lower] {
			return true
		}
	}
	return false
}

func parseDictionary(data []byte) (*Dictionary, error) {
	var d Dictionary
	return &d, json.Unmarshal(data, &d)
}

func foldString(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		b.WriteRune(unicode.ToLower(r))
		s = s[size:]
	}
	return b.String()
}
