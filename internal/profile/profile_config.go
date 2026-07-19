package profile

import (
	"fmt"
	"strings"

	"github.com/sdougbrown/writetighter/internal/config"
)

// ResolveTerm looks up a term with project-term overlay precedence.
//
// Precedence:
//  1. If a project term matches exactly (case-insensitive) with override=true
//     and a non-empty reason, it overrides any profile entry — return it.
//  2. If a project term matches exactly without override, it is a non-conflicting
//     addition — return it only if there is no conflicting discouraged profile entry.
//  3. If no project term matches, fall back to the dictionary lookup.
func ResolveTerm(dict *Dictionary, terms []config.TermEntry, query string) *Entry {
	queryFolded := foldString(query)

	// Check project terms for an exact case-insensitive match.
	for _, te := range terms {
		if foldString(te.Term) != queryFolded {
			continue
		}
		// Exact project term found.
		if te.Override && te.Reason != "" {
			// Override: always wins regardless of profile status.
			return &Entry{
				Term:          te.Term,
				PartsOfSpeech: te.PartsOfSpeech,
				Status:        StatusPreferred,
				Reason:        te.Reason,
			}
		}
		// Non-override project term: check for conflict with discouraged profile entry.
		if dict != nil {
			if prof := dict.index[queryFolded]; prof != nil && prof.Status == StatusDiscouraged {
				// Conflict — project term conflicts with discouraged profile entry
				// but does not have override+reason. Return nil so callers treat
				// the term as unknown/discouraged per profile policy.
				return nil
			}
		}
		// No conflict: return the project term as an additional entry.
		return &Entry{
			Term:          te.Term,
			PartsOfSpeech: te.PartsOfSpeech,
			Status:        StatusAllowed,
		}
	}

	// No project term match — fall through to dictionary lookup.
	if dict != nil {
		return dict.index[queryFolded]
	}
	return nil
}

// ValidateAgainstProfile checks project term entries against the active profile's
// dictionary. Returns an error if a project term conflicts with a discouraged
// profile entry but does not have override=true with a reason.
func ValidateAgainstProfile(entries []config.TermEntry, dict *Dictionary) error {
	if dict == nil || len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		if entry.Override && entry.Reason != "" {
			continue // overrides are allowed even against discouraged entries
		}
		if prof := dict.Lookup(entry.Term); prof != nil && prof.Status == StatusDiscouraged {
			return fmt.Errorf("term %q conflicts with profile discouraged entry %q and requires override=true with reason", entry.Term, prof.Term)
		}
	}
	return nil
}

// _ = strings used to keep import; we use foldString from dictionary.go
var _ = strings.TrimSpace
