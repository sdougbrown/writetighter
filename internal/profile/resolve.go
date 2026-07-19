package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var resolveRe = regexp.MustCompile(`^(?P<id>[a-z][a-z0-9_-]*)@(?P<ver>[0-9]+\.[0-9]+\.[0-9]+)$`)

func Resolve(spec string) (*Resolution, error) {
	if spec == "" {
		return LoadEmbedded()
	}
	if !resolveRe.MatchString(spec) {
		return nil, fmt.Errorf("invalid profile spec")
	}
	m := resolveRe.FindStringSubmatch(spec)
	installed := filepath.Join(profileRoot(), m[1], m[2])
	if embedded, err := LoadEmbedded(); err == nil && string(embedded.ID)+"@"+string(embedded.Version) == spec {
		if candidate, loadErr := LoadBundle(installed); loadErr == nil && candidate.SHA256 != embedded.SHA256 {
			return nil, profileConflictErr(m[1], m[2])
		} else if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
			return nil, fmt.Errorf("invalid installed profile %s: %w", spec, loadErr)
		}
		return embedded, nil
	}
	if r, err := LoadBundle(installed); err == nil {
		return r, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("invalid installed profile %s: %w", spec, err)
	}
	return nil, fmt.Errorf("profile not found")
}

func ListInstalled() ([]*Resolution, error) {
	root := profileRoot()
	ids, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Resolution
	for _, id := range ids {
		if !id.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(root, id.Name()))
		if err != nil {
			return nil, err
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			r, err := LoadBundle(filepath.Join(root, id.Name(), version.Name()))
			if err != nil {
				return nil, fmt.Errorf("invalid installed profile %s@%s: %w", id.Name(), version.Name(), err)
			}
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return string(out[i].ID)+"@"+string(out[i].Version) < string(out[j].ID)+"@"+string(out[j].Version)
	})
	return out, nil
}

func profileRoot() string {
	if root := os.Getenv("XDG_DATA_HOME"); root != "" {
		return filepath.Join(root, "writetighter", "profiles")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "writetighter", "profiles")
}
