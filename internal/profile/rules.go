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
// Rules with empty sets reject all parameters (no v1 parameters defined).
var knownRuleParams = map[string]map[string]bool{
	"CORE.SENTENCE_LENGTH": {
		"description_max_words": true,
		"procedure_max_words":   true,
		"pr_max_words":          true,
	},
	"CORE.DENSE_PARAGRAPH": {
		"max_sentences": true,
		"max_words":     true,
	},
	"CORE.NOUN_STACK": {
		"min_stack_length": true,
	},
	"CORE.TERM_DISCOURAGED":       {},
	"CORE.TERM_CASE":              {},
	"CORE.TERM_UNKNOWN":           {},
	"CORE.TERM_CONSISTENCY":       {},
	"CORE.PROCEDURE_MULTI_ACTION": {},
	"CORE.GERUND_OPENER":          {},
	"CORE.CONTRACTION":            {},
	"CORE.BANNED_MODAL":           {},
	"CORE.LATIN_ABBREV":           {},
	"CORE.UNEXPANDED_ABBREV":      {},
	"CORE.TIME_ANCHOR":            {},
	"CORE.EXCLAMATION":            {},
	"CORE.ORDINAL_NUMERAL":        {},
	"CORE.PERCENT_STYLE":          {},
	"CORE.AMBIGUOUS_DATE":         {},
	"CORE.HEADING_CASE":           {},
	"CORE.GERUND_HEADING":         {},
	"CORE.HEADING_SKIP":           {},
	"CORE.HEADING_PUNCTUATION":    {},
	"CORE.SINGLE_ITEM_LIST":       {},
	"CORE.SEQUENTIAL_BULLET":      {},
	"CORE.CORPUS_NOVELTY":        {"min_repetition": true},
}

var knownRules = map[string]bool{
	"CORE.SENTENCE_LENGTH": true, "CORE.DENSE_PARAGRAPH": true, "CORE.TERM_DISCOURAGED": true,
	"CORE.TERM_CASE": true, "CORE.TERM_UNKNOWN": true, "CORE.TERM_CONSISTENCY": true, "CORE.PROCEDURE_MULTI_ACTION": true,
	"CORE.NOUN_STACK": true, "CORE.GERUND_OPENER": true,
	"CORE.CONTRACTION": true, "CORE.BANNED_MODAL": true, "CORE.LATIN_ABBREV": true, "CORE.UNEXPANDED_ABBREV": true,
	"CORE.TIME_ANCHOR": true, "CORE.EXCLAMATION": true, "CORE.ORDINAL_NUMERAL": true, "CORE.PERCENT_STYLE": true,
	"CORE.AMBIGUOUS_DATE": true, "CORE.HEADING_CASE": true, "CORE.GERUND_HEADING": true, "CORE.HEADING_SKIP": true,
	"CORE.HEADING_PUNCTUATION": true, "CORE.SINGLE_ITEM_LIST": true, "CORE.SEQUENTIAL_BULLET": true,
	"CORE.CORPUS_NOVELTY": true,
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
		if rule.Version != 1 {
			errs = append(errs, fmt.Errorf("rules[%d]: unsupported version %d for rule %q", i, rule.Version, rule.ID))
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
		} else if len(rule.Parameters) > 0 {
			// Rules not in knownRuleParams still reject parameters
			for k := range rule.Parameters {
				errs = append(errs, fmt.Errorf("rules[%d]: unknown parameter %q for rule %q", i, k, rule.ID))
			}
		}
		if rule.Enabled {
			var required []string
			switch rule.ID {
			case "CORE.SENTENCE_LENGTH":
				required = []string{"description_max_words", "procedure_max_words", "pr_max_words"}
			case "CORE.DENSE_PARAGRAPH":
				required = []string{"max_sentences", "max_words"}
			}
			for _, key := range required {
				if _, ok := rule.Parameters[key]; !ok {
					errs = append(errs, fmt.Errorf("rules[%d]: missing required parameter %q for rule %q", i, key, rule.ID))
				}
			}
		}
		// Rejection rule 5: Validate parameter values are positive integers.
		for k, v := range rule.Parameters {
			switch rule.ID {
			case "CORE.SENTENCE_LENGTH", "CORE.DENSE_PARAGRAPH", "CORE.NOUN_STACK":
				n, ok := toInt(v)
				if !ok {
					errs = append(errs, fmt.Errorf("rules[%d]: parameter %q must be an integer, got %T(%v)", i, k, v, v))
					continue
				}
				if n <= 0 {
					errs = append(errs, fmt.Errorf("rules[%d]: parameter %q must be a positive integer, got %d", i, k, n))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func parseRules(data []byte) (*RulesConfig, error) {
	var r RulesConfig
	return &r, json.Unmarshal(data, &r)
}
