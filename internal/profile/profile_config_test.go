package profile

import (
	"testing"

	"github.com/sdougbrown/writetighter/internal/config"
)

func TestResolveTerm(t *testing.T) {
	dict := &Dictionary{
		FormatVersion: 1,
		Entries: []Entry{
			{Term: "alright", PartsOfSpeech: []string{"adj"}, Status: StatusDiscouraged, Reason: "use all right", Alternatives: []string{"all right"}},
			{Term: "fine", PartsOfSpeech: []string{"adj"}, Status: StatusAllowed},
			{Term: "hello", PartsOfSpeech: []string{"n"}, Status: StatusPreferred},
		},
	}
	if err := dict.Validate(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		terms      []config.TermEntry
		query      string
		wantNil    bool
		wantStatus EntryStatus
	}{
		{
			name:       "project override overrides discouraged",
			terms:      []config.TermEntry{{Term: "alright", Override: true, Reason: "company style"}},
			query:      "alright",
			wantStatus: StatusPreferred,
		},
		{
			name:       "project non-override addition",
			terms:      []config.TermEntry{{Term: "newterm", PartsOfSpeech: []string{"n"}}},
			query:      "newterm",
			wantStatus: StatusAllowed,
		},
		{
			name:    "project non-override conflicts with discouraged returns nil",
			terms:   []config.TermEntry{{Term: "alright", PartsOfSpeech: []string{"adj"}}},
			query:   "alright",
			wantNil: true,
		},
		{
			name:       "project non-override no conflict returns project term",
			terms:      []config.TermEntry{{Term: "fine", PartsOfSpeech: []string{"adj"}}},
			query:      "fine",
			wantStatus: StatusAllowed,
		},
		{
			name:       "no project match falls through to dict",
			terms:      []config.TermEntry{{Term: "other", PartsOfSpeech: []string{"n"}}},
			query:      "hello",
			wantStatus: StatusPreferred,
		},
		{
			name:       "case-insensitive match",
			terms:      []config.TermEntry{{Term: "ALRIGHT", Override: true, Reason: "test"}},
			query:      "alright",
			wantStatus: StatusPreferred,
		},
		{
			name:       "dict lookup case-insensitive",
			terms:      []config.TermEntry{},
			query:      "HELLO",
			wantStatus: StatusPreferred,
		},
		{
			name:    "not found anywhere",
			terms:   []config.TermEntry{},
			query:   "nonexistent",
			wantNil: true,
		},
		{
			name:       "nil dict with project override",
			terms:      []config.TermEntry{{Term: "anything", Override: true, Reason: "test"}},
			query:      "anything",
			wantStatus: StatusPreferred,
		},
		{
			name:    "nil dict no terms",
			terms:   []config.TermEntry{},
			query:   "anything",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveTerm(dict, tt.terms, tt.query)
			if tt.wantNil {
				if got != nil {
					t.Errorf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want non-nil, got nil")
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestValidateAgainstProfile(t *testing.T) {
	dict := &Dictionary{
		FormatVersion: 1,
		Entries: []Entry{
			{Term: "alright", PartsOfSpeech: []string{"adj"}, Status: StatusDiscouraged, Reason: "use all right"},
			{Term: "fine", PartsOfSpeech: []string{"adj"}, Status: StatusAllowed},
		},
	}
	if err := dict.Validate(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		entries []config.TermEntry
		wantErr bool
	}{
		{
			name:    "empty entries ok",
			entries: []config.TermEntry{},
			wantErr: false,
		},
		{
			name:    "nil dict ok",
			entries: []config.TermEntry{{Term: "x"}},
			wantErr: false,
		},
		{
			name:    "allowed entry no conflict ok",
			entries: []config.TermEntry{{Term: "fine"}},
			wantErr: false,
		},
		{
			name:    "discouraged without override errors",
			entries: []config.TermEntry{{Term: "alright"}},
			wantErr: true,
		},
		{
			name:    "discouraged with override and reason ok",
			entries: []config.TermEntry{{Term: "alright", Override: true, Reason: "company style"}},
			wantErr: false,
		},
		{
			name:    "discouraged with override but no reason still errors on conflict check",
			entries: []config.TermEntry{{Term: "alright", Override: true}},
			wantErr: true, // override=true without reason means we don't skip the conflict check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgainstProfile(tt.entries, dict)
			if tt.wantErr && err == nil {
				t.Error("want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("want no error, got: %v", err)
			}
		})
	}
}
