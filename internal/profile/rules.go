package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func (rc *RulesConfig) Validate() error {
	var errs []error
	if rc.FormatVersion != 1 {
		errs = append(errs, fmt.Errorf("unsupported rules format_version %d", rc.FormatVersion))
	}
	for i, rule := range rc.Rules {
		if rule.ID == "" {
			errs = append(errs, fmt.Errorf("rules[%d]: missing rule ID", i))
			continue
		}
		// Rejection rule 5: Validate rule ID starts with "CORE." or "<PROFILE_ID>."
		if !strings.HasPrefix(rule.ID, "CORE.") {
			parts := strings.SplitN(rule.ID, ".", 2)
			if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
				errs = append(errs, fmt.Errorf("rules[%d]: invalid rule ID %q (must start with CORE. or PROFILE_ID.suffix)", i, rule.ID))
			}
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
