package profile

import "github.com/sdougbrown/writetighter/internal/config"

func LookupWithTerms(dict *Dictionary, terms []config.TermEntry) *Entry {
	if dict == nil {
		return nil
	}
	for _, term := range terms {
		if term.Override && term.Reason != "" {
			if e := dict.Lookup(term.Term); e != nil {
				return e
			}
		}
	}
	return nil
}
