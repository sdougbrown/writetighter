package profile

import (
	"encoding/json"
	"errors"
	"fmt"
)

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

// knownRuleParams defines valid parameter keys for known rule IDs.
var knownRuleParams = map[string]map[string]bool{
	"CORE.SENTENCE_LENGTH": {
		"description_max_words": true,
		"procedure_max_words":   true,
		"pr_max_words":          true,
	},
}

var knownRules = map[string]bool{
	"CORE.SENTENCE_LENGTH": true, "CORE.DENSE_PARAGRAPH": true, "CORE.TERM_DISCOURAGED": true,
	"CORE.TERM_CASE": true, "CORE.TERM_UNKNOWN": true, "CORE.TERM_CONSISTENCY": true, "CORE.PROCEDURE_MULTI_ACTION": true,
}

func (rc *RulesConfig) Validate() error {
	var errs []error
	if rc.FormatVersion != 1 {
		errs = append(errs, fmt.Errorf("unsupported rules format_version %d", rc.FormatVersion))
	}
	seen := map[string]bool{}
	for i, rule := range rc.Rules {
		if rule.ID == "" {
			errs = append(errs, fmt.Errorf("rules[%d]: missing rule ID", i))
			continue
		}
		if !knownRules[rule.ID] {
			errs = append(errs, fmt.Errorf("rules[%d]: unsupported rule ID %q", i, rule.ID))
		}
		if seen[rule.ID] {
			errs = append(errs, fmt.Errorf("rules[%d]: duplicate rule ID %q", i, rule.ID))
		}
		seen[rule.ID] = true
		if rule.Version < 1 {
			errs = append(errs, fmt.Errorf("rules[%d]: version must be positive", i))
		}
		if rule.Enforcement != "enforced" && rule.Enforcement != "candidate" && rule.Enforcement != "advisory" && rule.Enforcement != "disabled" {
			errs = append(errs, fmt.Errorf("rules[%d]: invalid enforcement", i))
		}
		if rule.Severity != "error" && rule.Severity != "warning" && rule.Severity != "info" {
			errs = append(errs, fmt.Errorf("rules[%d]: invalid severity", i))
		}
		// Rejection rule 5: Validate parameter keys for known rules
		if validParams, ok := knownRuleParams[rule.ID]; ok {
			for k := range rule.Parameters {
				if !validParams[k] {
					errs = append(errs, fmt.Errorf("rules[%d]: unknown parameter %q for rule %q", i, k, rule.ID))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func parseRules(data []byte) (*RulesConfig, error) {
	var r RulesConfig
	return &r, json.Unmarshal(data, &r)
}
