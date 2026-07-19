package profile

import (
	"os"
	"path/filepath"
)

type VerifyResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

func LoadBundle(dir string) (*Resolution, error) {
	m, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	d, _ := os.ReadFile(filepath.Join(dir, "dictionary.json"))
	r, _ := os.ReadFile(filepath.Join(dir, "rules.json"))
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
