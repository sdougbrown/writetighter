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
type Provenance struct{ Name, Release, SourceURL, License string }
type ManifestClaims struct {
	Standard      *string `json:"standard"`
	Issue         *string `json:"issue"`
	Certification string  `json:"certification"`
}
type Payloads struct {
	DictionarySHA256 PayloadHash `json:"dictionary.json"`
	RulesSHA256      PayloadHash `json:"rules.json"`
}

type PayloadHash struct {
	SHA256 string `json:"sha256"`
}

func (m *Manifest) Validate() error {
	var errs []error
	if m.FormatVersion != 1 {
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
	if m.Payloads.DictionarySHA256.SHA256 == "" || m.Payloads.RulesSHA256.SHA256 == "" {
		errs = append(errs, fmt.Errorf("manifest missing payload hashes"))
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
		errs = append(errs, fmt.Errorf("manifest requires at least one provenance entry"))
	}
	for i, p := range m.Provenance {
		if p.Name == "" {
			errs = append(errs, fmt.Errorf("provenance[%d]: name is required", i))
		}
	}
	return errors.Join(errs...)
}

func parseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	return &m, json.Unmarshal(data, &m)
}
