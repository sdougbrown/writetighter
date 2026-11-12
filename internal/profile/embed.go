package profile

import (
	"embed"
	"fmt"
)

//go:embed data/profiles/software-docs-en/0.7.0/manifest.json
//go:embed data/profiles/software-docs-en/0.7.0/lexicon.json
//go:embed data/profiles/software-docs-en/0.7.0/rules.json
//go:embed data/dictionaries/*.json
var embeddedFiles embed.FS

const embeddedReleaseDir = "data/profiles/software-docs-en/0.7.0"

func loadEmbeddedBundle() (*Resolution, error) {
	mBytes, err := embeddedFiles.ReadFile(embeddedReleaseDir + "/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("reading embedded manifest: %w", err)
	}
	l, err := embeddedFiles.ReadFile(embeddedReleaseDir + "/lexicon.json")
	if err != nil {
		return nil, fmt.Errorf("reading embedded lexicon: %w", err)
	}
	r, err := embeddedFiles.ReadFile(embeddedReleaseDir + "/rules.json")
	if err != nil {
		return nil, fmt.Errorf("reading embedded rules: %w", err)
	}
	m, err := parseManifest(mBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing embedded manifest: %w", err)
	}
	return assembleBundle("embedded", m, mBytes, l, r, nil, resolveEmbeddedDictionary)
}

// resolveEmbeddedDictionary finds the content-addressed dictionary whose
// bytes hash to sha in the embedded dictionaries store.
func resolveEmbeddedDictionary(sha string) ([]byte, error) {
	entries, err := embeddedFiles.ReadDir("data/dictionaries")
	if err != nil {
		return nil, fmt.Errorf("reading embedded dictionary store: %w", err)
	}
	for _, e := range entries {
		b, err := embeddedFiles.ReadFile("data/dictionaries/" + e.Name())
		if err != nil {
			continue
		}
		if SHA256Bytes(b) == sha {
			return b, nil
		}
	}
	return nil, fmt.Errorf("no embedded dictionary matches sha256 %s", sha)
}
