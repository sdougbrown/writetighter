package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

type VerifyResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

func LoadBundle(dir string) (*Resolution, error) {
	m, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	d, err := os.ReadFile(filepath.Join(dir, "dictionary.json"))
	if err != nil {
		return nil, fmt.Errorf("reading dictionary: %w", err)
	}
	r, err := os.ReadFile(filepath.Join(dir, "rules.json"))
	if err != nil {
		return nil, fmt.Errorf("reading rules: %w", err)
	}
	return loadBundleFromBytes(dir, m, d, r)
}
func LoadEmbedded() (*Resolution, error) { return loadEmbeddedBundle() }
func ValidateBundle(dir string) error    { _, err := LoadBundle(dir); return err }
func VerifyBundle(dir string) *VerifyResult {
	if err := ValidateBundle(dir); err != nil {
		return &VerifyResult{Valid: false, Errors: []string{err.Error()}}
	}
	return &VerifyResult{Valid: true}
}
