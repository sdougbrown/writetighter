package profile

import (
	"encoding/json"
	"fmt"
	"regexp"
)

var (
	profileIDRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	versionRe   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
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
	if m.FormatVersion != 1 {
		return fmt.Errorf("unsupported manifest format_version %d", m.FormatVersion)
	}
	if !profileIDRe.MatchString(m.ID) {
		return fmt.Errorf("invalid profile id %q", m.ID)
	}
	if !versionRe.MatchString(m.Version) {
		return fmt.Errorf("invalid version %q", m.Version)
	}
	if m.DisplayName == "" || m.Language == "" || m.License.SPDX == "" {
		return fmt.Errorf("manifest missing required fields")
	}
	if m.Payloads.DictionarySHA256.SHA256 == "" || m.Payloads.RulesSHA256.SHA256 == "" {
		return fmt.Errorf("manifest missing payload hashes")
	}
	return nil
}

func parseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	return &m, json.Unmarshal(data, &m)
}
