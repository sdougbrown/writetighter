package profile

import (
	"embed"
	"fmt"
)

//go:embed data/profiles/software-docs-en/0.1.0/manifest.json
//go:embed data/profiles/software-docs-en/0.1.0/dictionary.json
//go:embed data/profiles/software-docs-en/0.1.0/rules.json
var embeddedFiles embed.FS

func loadEmbeddedBundle() (*Resolution, error) {
	m, _ := embeddedFiles.ReadFile("data/profiles/software-docs-en/0.1.0/manifest.json")
	d, _ := embeddedFiles.ReadFile("data/profiles/software-docs-en/0.1.0/dictionary.json")
	r, _ := embeddedFiles.ReadFile("data/profiles/software-docs-en/0.1.0/rules.json")
	return loadBundleFromBytes("embedded", m, d, r)
}

func loadBundleFromBytes(source string, manifestBytes, dictBytes, rulesBytes []byte) (*Resolution, error) {
	m, err := parseManifest(manifestBytes)
	if err != nil {
		return nil, err
	}
	d, err := parseDictionary(dictBytes)
	if err != nil {
		return nil, err
	}
	r, err := parseRules(rulesBytes)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if got := SHA256Bytes(dictBytes); got != m.Payloads.DictionarySHA256.SHA256 {
		return nil, fmt.Errorf("dictionary hash mismatch: got %s want %s", got, m.Payloads.DictionarySHA256.SHA256)
	}
	if got := SHA256Bytes(rulesBytes); got != m.Payloads.RulesSHA256.SHA256 {
		return nil, fmt.Errorf("rules hash mismatch: got %s want %s", got, m.Payloads.RulesSHA256.SHA256)
	}
	sha := SHA256Bytes(append(append(manifestBytes, dictBytes...), rulesBytes...))
	return &Resolution{ID: ProfileID(m.ID), Version: Version(m.Version), SHA256: sha, Source: source, Manifest: m, Dict: d, Rules: r}, nil
}
