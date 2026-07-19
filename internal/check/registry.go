package check

import "github.com/sdougbrown/writetighter/internal/profile"

var registry = map[string]Checker{}

func Register(c Checker) {
	registry[c.ID()] = c
}

func Get(id string) Checker { return registry[id] }

func Enabled(profile *profile.Resolution) []Checker {
	var result []Checker
	for _, rule := range profile.Rules.Rules {
		if !rule.Enabled {
			continue
		}
		if c := Get(rule.ID); c != nil {
			result = append(result, c)
		}
	}
	return result
}

func All() []Checker {
	result := make([]Checker, 0, len(registry))
	for _, c := range registry {
		result = append(result, c)
	}
	return result
}
