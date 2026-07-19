package profile

import "encoding/json"

type RulesConfig struct {
	FormatVersion     int    `json:"format_version"`
	UnknownTermPolicy string `json:"unknown_term_policy"`
	Rules             []Rule `json:"rules"`
}
type Rule struct {
	ID          string         `json:"id"`
	Version     int            `json:"version"`
	Enabled     bool           `json:"enabled"`
	Enforcement string         `json:"enforcement"`
	Severity    string         `json:"severity"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

func parseRules(data []byte) (*RulesConfig, error) {
	var r RulesConfig
	return &r, json.Unmarshal(data, &r)
}
