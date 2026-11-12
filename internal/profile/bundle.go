package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type VerifyResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

func LoadBundle(dir string) (*Resolution, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	m, err := parseManifest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	layout, ok := layoutFor(m.FormatVersion)
	if !ok {
		return nil, fmt.Errorf("unsupported manifest format_version %d", m.FormatVersion)
	}
	var rulesBytes, lexiconBytes, dictBytes []byte
	slots := map[string]*[]byte{
		"rules.json":      &rulesBytes,
		"lexicon.json":    &lexiconBytes,
		"dictionary.json": &dictBytes,
	}
	for _, name := range layout.releaseFiles {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		*slots[name] = b
	}
	if len(layout.materialized) > 0 {
		// The corpus-derived dictionary is content-addressed. A materialized
		// copy next to the manifest (as written by InstallBundle) wins; the
		// shared embedded store resolves it otherwise, so a repo checkout
		// validates in place. Any other read failure is fatal: silently
		// resolving from the store would mask a broken installed bundle.
		if b, readErr := os.ReadFile(filepath.Join(dir, "dictionary.json")); readErr == nil {
			dictBytes = b
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return nil, fmt.Errorf("reading dictionary: %w", readErr)
		}
	}

	return assembleBundle(dir, m, manifestBytes, lexiconBytes, rulesBytes, dictBytes, resolveEmbeddedDictionary)
}

// assembleBundle validates and assembles a bundle from its payload bytes.
// The manifest pins every payload by sha256. The dictionary arrives inline
// (format 1/2) or is resolved from the content-addressed store by the caller
// (format 3, dictBytes nil + resolver).
func assembleBundle(source string, m *Manifest, manifestBytes, lexiconBytes, rulesBytes, dictBytes []byte, resolveDict func(sha string) ([]byte, error)) (*Resolution, error) {
	r, err := parseRules(rulesBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing rules: %w", err)
	}

	// Run all validation, collecting ALL errors
	var errs []error

	if err := m.Validate(); err != nil {
		errs = append(errs, err)
	}

	// Resolve the dictionary payload for format 3 when the caller did not
	// hand us bytes. A missing payload hash is reported by
	// Manifest.validatePayloads below; here it just skips resolution.
	dictName := m.DictionaryPayloadName()
	if dictBytes == nil && m.FormatVersion >= 3 {
		sha := m.PayloadSHA256(dictName)
		if sha != "" {
			if resolveDict == nil {
				errs = append(errs, fmt.Errorf("no dictionary resolver available for %q", dictName))
			} else {
				dictBytes, err = resolveDict(sha)
				if err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	// Parse the dictionary (format 1/2: entries + word classes + suggestion
	// tables; format 3: entries only) and merge the curated lexicon over it.
	var d *Dictionary
	if dictBytes != nil {
		d, err = parseDictionary(dictBytes)
		if err != nil {
			errs = append(errs, fmt.Errorf("parsing dictionary: %w", err))
		} else if lexiconBytes != nil {
			lex, err := parseLexicon(lexiconBytes)
			if err != nil {
				errs = append(errs, fmt.Errorf("parsing lexicon: %w", err))
			} else if lex.FormatVersion != 1 {
				errs = append(errs, fmt.Errorf("unsupported lexicon format_version %d", lex.FormatVersion))
			} else {
				d.WordClasses = lex.WordClasses
				d.BannedModalSuggestions = lex.BannedModalSuggestions
				d.BannedLatinAbbrevSuggestions = lex.BannedLatinAbbrevSuggestions
			}
		}
		if d != nil {
			if err := d.Validate(); err != nil {
				errs = append(errs, err)
			}
			// Rejection rule 7: Alternatives resolution
			if err := d.ValidateAlternatives(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// Rules validation
	if err := r.Validate(); err != nil {
		errs = append(errs, err)
	}

	// Hash validation (payload hash check against actual bytes)
	checkHash := func(name string, bytes []byte) {
		if bytes == nil {
			return
		}
		if want := m.PayloadSHA256(name); want != "" {
			if got := SHA256Bytes(bytes); got != want {
				errs = append(errs, fmt.Errorf("%s hash mismatch: got %s want %s", name, got, want))
			}
		}
	}
	checkHash("rules.json", rulesBytes)
	checkHash("lexicon.json", lexiconBytes)
	checkHash(dictName, dictBytes)

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Bundle resolution hash over the canonical payload order (manifest,
	// dictionary, lexicon, rules); empty slices are skipped so format 1/2
	// bundles hash exactly as they did before format 3 existed.
	sha := SHA256Bytes(concat(manifestBytes, dictBytes, lexiconBytes, rulesBytes))
	return &Resolution{ID: ProfileID(m.ID), Version: Version(m.Version), SHA256: sha, Source: source, Manifest: m, dictBytes: dictBytes, Dict: d, Rules: r}, nil
}

func concat(slices ...[]byte) []byte {
	var out []byte
	for _, s := range slices {
		if len(s) == 0 {
			continue
		}
		out = append(out, s...)
	}
	return out
}

// Lexicon is the curated language layer of a profile: word classes and
// suggestion tables. It sits alongside rules.json in the release directory;
// the corpus-derived dictionary is referenced from the shared store.
type Lexicon struct {
	FormatVersion                int                 `json:"format_version"`
	WordClasses                  map[string][]string `json:"word_classes,omitempty"`
	BannedModalSuggestions       map[string]string   `json:"banned_modal_suggestions,omitempty"`
	BannedLatinAbbrevSuggestions map[string]string   `json:"banned_latin_abbrev_suggestions,omitempty"`
}

func parseLexicon(data []byte) (*Lexicon, error) {
	var l Lexicon
	return &l, json.Unmarshal(data, &l)
}

func LoadEmbedded() (*Resolution, error) { return loadEmbeddedBundle() }
func ValidateBundle(dir string) error    { _, err := LoadBundle(dir); return err }
func VerifyBundle(dir string) *VerifyResult {
	if err := ValidateBundle(dir); err != nil {
		return &VerifyResult{Valid: false, Errors: []string{err.Error()}}
	}
	return &VerifyResult{Valid: true}
}
