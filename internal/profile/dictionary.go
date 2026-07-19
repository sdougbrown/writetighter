package profile

import (
	"encoding/json"
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
	if d.FormatVersion != 1 {
		return fmt.Errorf("unsupported dictionary format_version %d", d.FormatVersion)
	}
	d.index = map[string]*Entry{}
	for i := range d.Entries {
		e := &d.Entries[i]
		if e.Term == "" {
			return fmt.Errorf("entry %d: empty term", i)
		}
		if e.Status == StatusPreferred || e.Status == StatusAllowed || e.Status == StatusDiscouraged {
			if len(e.PartsOfSpeech) == 0 {
				return fmt.Errorf("entry %q: status %s requires parts_of_speech", e.Term, e.Status)
			}
		}
		if e.Status == StatusDiscouraged && strings.TrimSpace(e.Reason) == "" {
			return fmt.Errorf("entry %q: discouraged requires reason", e.Term)
		}
		k := foldString(e.Term)
		if _, ok := d.index[k]; ok {
			return fmt.Errorf("duplicate term %q", e.Term)
		}
		d.index[k] = e
	}
	return nil
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
