package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

func InstallBundle(dir string) (*Resolution, error) {
	r, err := LoadBundle(dir)
	if err != nil {
		return nil, fmt.Errorf("invalid bundle: %w", err)
	}
	target := filepath.Join(profileRoot(), string(r.ID), string(r.Version))
	if embedded, err := LoadEmbedded(); err == nil && embedded.ID == r.ID && embedded.Version == r.Version {
		if embedded.SHA256 == r.SHA256 {
			return r, nil
		}
		return nil, profileConflictErr(string(r.ID), string(r.Version))
	}
	if existing, err := LoadBundle(target); err == nil {
		if existing.SHA256 == r.SHA256 {
			return r, nil
		}
		return nil, profileConflictErr(string(r.ID), string(r.Version))
	}
	if _, err := os.Stat(target); err == nil {
		return nil, fmt.Errorf("invalid existing profile %s@%s", r.ID, r.Version)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, fmt.Errorf("creating profile parent: %w", err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), "."+string(r.Version)+".tmp-")
	if err != nil {
		return nil, fmt.Errorf("creating temp: %w", err)
	}
	defer os.RemoveAll(tmp)
	if err := copyBundle(dir, tmp, r); err != nil {
		return nil, fmt.Errorf("copying: %w", err)
	}
	if _, err := LoadBundle(tmp); err != nil {
		return nil, fmt.Errorf("temp bundle invalid: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		if existing, loadErr := LoadBundle(target); loadErr == nil && existing.SHA256 == r.SHA256 {
			return r, nil
		}
		return nil, fmt.Errorf("rename: %w", err)
	}
	return r, nil
}

// copyBundle materializes the resolved bundle into dst. Format 3 source
// directories reference the content-addressed dictionary rather than carry
// it, so the resolved bytes are written alongside the manifest — the
// installed bundle is self-contained and LoadBundle needs no resolver.
func copyBundle(src, dst string, r *Resolution) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	files := []string{"manifest.json", "rules.json"}
	if r.Manifest.FormatVersion >= 3 {
		files = append(files, "lexicon.json", "dictionary.json")
	} else {
		files = append(files, "dictionary.json")
	}
	for _, n := range files {
		var b []byte
		var err error
		if n == "dictionary.json" && r.Manifest.FormatVersion >= 3 {
			// Resolved bytes, not a source file: hash-pinned in the manifest.
			b, err = resolveDictionaryForInstall(r)
		} else {
			b, err = os.ReadFile(filepath.Join(src, n))
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, n), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// resolveDictionaryForInstall re-resolves the content-addressed dictionary
// for an already-validated resolution. It reads the materialized copy when
// present and otherwise falls back to the embedded store, mirroring
// LoadBundle's resolution order.
func resolveDictionaryForInstall(r *Resolution) ([]byte, error) {
	if r.Source != "" {
		if b, err := os.ReadFile(filepath.Join(r.Source, "dictionary.json")); err == nil {
			return b, nil
		}
	}
	sha := r.Manifest.PayloadSHA256(r.Manifest.DictionaryPayloadName())
	b, err := resolveEmbeddedDictionary(sha)
	if err != nil {
		return nil, fmt.Errorf("resolving dictionary for install: %w", err)
	}
	return b, nil
}

func profileConflictErr(id, version string) error {
	return fmt.Errorf("profile %s@%s conflict", id, version)
}
