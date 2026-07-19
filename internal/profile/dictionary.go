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
	FormatVersion int     `json:"format_version"`
	Entries       []Entry `json:"entries"`
	index         map[string]*Entry
}

func (d *Dictionary) Validate() error {
	var errs []error
	if d.FormatVersion != 1 {
		errs = append(errs, fmt.Errorf("unsupported dictionary format_version %d", d.FormatVersion))
	}
	d.index = map[string]*Entry{}
	for i := range d.Entries {
		e := &d.Entries[i]
		if e.Term == "" {
			errs = append(errs, fmt.Errorf("entry %d: empty term", i))
			continue
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
