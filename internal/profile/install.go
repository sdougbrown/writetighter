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
	base, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	target := filepath.Join(base, ".local", "share", "writetighter", "profiles", string(r.ID), string(r.Version))
	if existing, err := LoadBundle(target); err == nil {
		if existing.SHA256 == r.SHA256 {
			return r, nil
		}
		return nil, profileConflictErr(string(r.ID), string(r.Version))
	}
	tmp := target + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return nil, fmt.Errorf("removing temp: %w", err)
	}
	if err := copyDir(dir, tmp); err != nil {
		return nil, fmt.Errorf("copying: %w", err)
	}
	if _, err := LoadBundle(tmp); err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("temp bundle invalid: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("rename: %w", err)
	}
	return r, nil
}
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	for _, n := range []string{"manifest.json", "dictionary.json", "rules.json"} {
		b, err := os.ReadFile(filepath.Join(src, n))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, n), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
func profileConflictErr(id, version string) error {
	return fmt.Errorf("profile %s@%s conflict", id, version)
}
