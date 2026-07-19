package profile

import (
	"fmt"
	"regexp"
)

var resolveRe = regexp.MustCompile(`^(?P<id>[a-z][a-z0-9_-]*)@(?P<ver>[0-9]+\.[0-9]+\.[0-9]+)$`)

func Resolve(spec string) (*Resolution, error) {
	if spec == "" {
		return LoadEmbedded()
	}
	if !resolveRe.MatchString(spec) {
		return nil, fmt.Errorf("invalid profile spec")
	}
	if r, err := LoadEmbedded(); err == nil && string(r.ID)+"@"+string(r.Version) == spec {
		return r, nil
	}
	return nil, fmt.Errorf("profile not found")
}
