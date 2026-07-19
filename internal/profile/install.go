package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

func InstallBundle(dir string) (*Resolution, error) {
	r, err := LoadBundle(dir)
	if err != nil {
		return nil, err
	}
	base, _ := os.UserHomeDir()
	target := filepath.Join(base, ".local", "share", "writetighter", "profiles", string(r.ID), string(r.Version))
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	if _, err := os.Stat(target); err == nil {
		return r, nil
	}
	tmp := target + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := copyDir(dir, tmp); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, target); err != nil {
		return nil, err
	}
	return r, nil
}
func copyDir(src, dst string) error {
	_ = os.MkdirAll(dst, 0o755)
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
