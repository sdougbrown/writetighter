package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var (
	profileIDRe    = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	versionRe      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	supportedKinds = map[string]bool{
		"description": true,
		"procedure":   true,
		"pr":          true,
	}
)

// Manifest format versions:
//
//	1, 2 — the release directory is self-contained: dictionary.json and
//	rules.json sit next to manifest.json and the payloads key names them.
//	3 — the corpus-derived dictionary is content-addressed in the shared
//	dictionaries store (payload key "dictionary", matched by sha256); the
//	release directory carries only rules.json and the curated lexicon.json.
type Manifest struct {
	FormatVersion  int             `json:"format_version"`
	ID             string          `json:"id"`
	Version        string          `json:"version"`
	DisplayName    string          `json:"display_name"`
	Language       string          `json:"language"`
	SupportedKinds []string        `json:"supported_kinds,omitempty"`
	License        ManifestLicense `json:"license"`
	Provenance     []Provenance    `json:"provenance,omitempty"`
	Claims         ManifestClaims  `json:"claims,omitempty"`
	Payloads       Payloads        `json:"payloads"`
}

type ManifestLicense struct {
	SPDX string `json:"spdx"`
}
type Provenance struct {
	Name      string `json:"name"`
	Release   string `json:"release"`
	SourceURL string `json:"source_url"`
	License   string `json:"license"`
}
type ManifestClaims struct {
	Standard      *string `json:"standard"`
	Issue         *string `json:"issue"`
	Certification string  `json:"certification"`
}

// Payloads maps a payload name to its content hash. Format 1/2 manifests use
// the file names ("dictionary.json", "rules.json"); format 3 manifests use
// "rules.json", "lexicon.json", and the content-addressed "dictionary".
type Payloads map[string]PayloadHash

type PayloadHash struct {
	SHA256 string `json:"sha256"`
}

// PayloadSHA256 returns the pinned hash for a payload name, or "" when the
// manifest does not carry it.
func (m *Manifest) PayloadSHA256(name string) string {
	if m == nil || m.Payloads == nil {
		return ""
	}
	return m.Payloads[name].SHA256
}

// DictionaryPayloadName reports the payload key that pins the dictionary for
// this manifest's format version: "dictionary.json" (inline, format 1/2) or
// "dictionary" (content-addressed reference, format 3).
func (m *Manifest) DictionaryPayloadName() string {
	if m.FormatVersion >= 3 {
		return "dictionary"
	}
	return "dictionary.json"
}

func (m *Manifest) Validate() error {
	var errs []error
	switch m.FormatVersion {
	case 1, 2, 3:
	default:
		errs = append(errs, fmt.Errorf("unsupported manifest format_version %d", m.FormatVersion))
	}
	if !profileIDRe.MatchString(m.ID) {
		errs = append(errs, fmt.Errorf("invalid profile id %q", m.ID))
	}
	if !versionRe.MatchString(m.Version) {
		errs = append(errs, fmt.Errorf("invalid version %q", m.Version))
	}
	if m.DisplayName == "" || m.Language == "" || m.License.SPDX == "" {
		errs = append(errs, fmt.Errorf("manifest missing required fields (display_name, language, license.spdx)"))
	}
	if len(m.SupportedKinds) == 0 {
		errs = append(errs, errors.New("manifest requires supported_kinds"))
	}
	if m.Claims.Certification != "none" {
		errs = append(errs, errors.New("manifest certification must be none"))
	}
	if err := m.validatePayloads(); err != nil {
		errs = append(errs, err)
	}
	// Rejection rule 8: Unsupported language
	if m.Language != "" && m.Language != "en" {
		errs = append(errs, fmt.Errorf("unsupported language %q (only \"en\" is supported in v1)", m.Language))
	}
	// Rejection rule 8: Unsupported document kinds
	for _, k := range m.SupportedKinds {
		if !supportedKinds[k] {
			errs = append(errs, fmt.Errorf("unsupported document kind %q (must be description, procedure, or pr)", k))
		}
	}
	// Rejection rule 9: Absent license/provenance
	if len(m.Provenance) == 0 {
		errs = append(errs, errors.New("manifest requires at least one provenance entry"))
	}
	for i, p := range m.Provenance {
		if p.Name == "" || p.Release == "" || p.SourceURL == "" || p.License == "" {
			errs = append(errs, fmt.Errorf("provenance[%d]: name, release, source_url, and license are required", i))
		}
	}
	return errors.Join(errs...)
}

// bundleLayout owns the file layout of each manifest format version: the
// payloads the manifest must pin, the files a release directory carries,
// and the files install materializes from resolved bytes. Adding a format
// version or renaming a payload edits this table and nothing else.
type bundleLayout struct {
	requiredPayloads []string
	releaseFiles     []string
	materialized     []string
}

func layoutFor(formatVersion int) (bundleLayout, bool) {
	switch formatVersion {
	case 1, 2:
		return bundleLayout{
			requiredPayloads: []string{"dictionary.json", "rules.json"},
			releaseFiles:     []string{"dictionary.json", "rules.json"},
		}, true
	case 3:
		return bundleLayout{
			requiredPayloads: []string{"dictionary", "lexicon.json", "rules.json"},
			releaseFiles:     []string{"lexicon.json", "rules.json"},
			materialized:     []string{"dictionary.json"},
		}, true
	}
	return bundleLayout{}, false
}

func (m *Manifest) validatePayloads() error {
	layout, ok := layoutFor(m.FormatVersion)
	if !ok {
		return fmt.Errorf("unsupported manifest format_version %d", m.FormatVersion)
	}
	var errs []error
	for _, name := range layout.requiredPayloads {
		if m.PayloadSHA256(name) == "" {
			errs = append(errs, fmt.Errorf("manifest missing payload hash for %q", name))
		}
	}
	for name := range m.Payloads {
		known := false
		for _, r := range layout.requiredPayloads {
			if name == r {
				known = true
			}
		}
		if !known {
			errs = append(errs, fmt.Errorf("manifest carries unknown payload %q for format_version %d", name, m.FormatVersion))
		}
	}
	return errors.Join(errs...)
}

func parseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	return &m, json.Unmarshal(data, &m)
}
