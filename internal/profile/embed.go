package profile

import (
	"embed"
	"errors"
	"fmt"
)

//go:embed data/profiles/software-docs-en/0.4.0/manifest.json
//go:embed data/profiles/software-docs-en/0.4.0/dictionary.json
//go:embed data/profiles/software-docs-en/0.4.0/rules.json
var embeddedFiles embed.FS

func loadEmbeddedBundle() (*Resolution, error) {
	m, _ := embeddedFiles.ReadFile("data/profiles/software-docs-en/0.4.0/manifest.json")
	d, _ := embeddedFiles.ReadFile("data/profiles/software-docs-en/0.4.0/dictionary.json")
	r, _ := embeddedFiles.ReadFile("data/profiles/software-docs-en/0.4.0/rules.json")
	return loadBundleFromBytes("embedded", m, d, r)
}

func loadBundleFromBytes(source string, manifestBytes, dictBytes, rulesBytes []byte) (*Resolution, error) {
	m, err := parseManifest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	d, err := parseDictionary(dictBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing dictionary: %w", err)
	}
	r, err := parseRules(rulesBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing rules: %w", err)
	}

	// Run all validation, collecting ALL errors
	var errs []error

	// Manifest validation
	if err := m.Validate(); err != nil {
		errs = append(errs, err)
	}

	// Dictionary validation
	if err := d.Validate(); err != nil {
		errs = append(errs, err)
	}

	// Rules validation
	if err := r.Validate(); err != nil {
		errs = append(errs, err)
	}

	// Hash validation (payload hash check against actual bytes)
	if m.Payloads.DictionarySHA256.SHA256 != "" {
		if got := SHA256Bytes(dictBytes); got != m.Payloads.DictionarySHA256.SHA256 {
			errs = append(errs, fmt.Errorf("dictionary hash mismatch: got %s want %s", got, m.Payloads.DictionarySHA256.SHA256))
		}
	}
	if m.Payloads.RulesSHA256.SHA256 != "" {
		if got := SHA256Bytes(rulesBytes); got != m.Payloads.RulesSHA256.SHA256 {
			errs = append(errs, fmt.Errorf("rules hash mismatch: got %s want %s", got, m.Payloads.RulesSHA256.SHA256))
		}
	}

	// Rejection rule 7: Alternatives resolution
	if err := d.ValidateAlternatives(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	sha := SHA256Bytes(append(append(manifestBytes, dictBytes...), rulesBytes...))
	return &Resolution{ID: ProfileID(m.ID), Version: Version(m.Version), SHA256: sha, Source: source, Manifest: m, Dict: d, Rules: r}, nil
}
