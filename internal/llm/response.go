package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/report"
)

type AdvisorResponse struct {
	Findings []AdvisorFinding `json:"findings"`
}

type AdvisorFinding struct {
	SourceRange SourceRange `json:"source_range"`
	RuleIDs     []string    `json:"rule_ids"`
	Reason      string      `json:"reason"`
	Replacement string      `json:"replacement"`
	Confidence  float64     `json:"confidence"`
}

type SourceRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ValidateAdvisorResponse validates LLM JSON output with default rule ID checking.
func ValidateAdvisorResponse(raw []byte, input string) ([]report.Finding, error) {
	return ValidateAdvisorResponseForRules(raw, input, nil)
}

// ValidateAdvisorResponseForRules validates LLM JSON output and enforces that
// each rule ID is in the active set (or passes prefix checks when active is nil).
//
// Validation rules:
//   - Exactly one JSON value with only trailing whitespace allowed.
//   - Response size <= MaxOutputChars.
//   - At most MaxSuggestions findings.
//   - Source ranges are valid: start >= 0, end >= start, end <= len(input).
//   - Source range boundaries are at valid UTF-8 code point boundaries.
//     (U+FFFD is a valid code point and must not be rejected.)
//   - Rule IDs are present and either in active set or have valid prefix.
//   - Replacement/reason must not contain claim language.
//   - Confidence in [0, 1].
//   - Disallow unknown JSON fields.
func ValidateAdvisorResponseForRules(raw []byte, input string, active map[string]struct{}) ([]report.Finding, error) {
	if len(raw) > MaxOutputChars {
		return nil, errors.New("llm response too large")
	}
	// Require exactly one JSON value with only trailing whitespace.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var resp AdvisorResponse
	if err := dec.Decode(&resp); err != nil {
		return nil, err
	}
	// Check that there's nothing after the JSON value except whitespace.
	remaining := dec.InputOffset()
	if remaining < int64(len(raw)) {
		trailing := string(raw[remaining:])
		if strings.TrimSpace(trailing) != "" {
			return nil, fmt.Errorf("trailing content after JSON value")
		}
	}
	if len(resp.Findings) > MaxSuggestions {
		return nil, errors.New("too many llm suggestions")
	}
	var out []report.Finding
	for _, f := range resp.Findings {
		if f.SourceRange.Start < 0 || f.SourceRange.End < f.SourceRange.Start || f.SourceRange.End > len(input) {
			return nil, fmt.Errorf("invalid source range [%d, %d)", f.SourceRange.Start, f.SourceRange.End)
		}
		// Validate UTF-8 code point boundaries.
		// A byte position is a valid boundary if it is at position 0, at len(input),
		// or if the byte at that position is a rune start (i.e., not a continuation byte).
		if f.SourceRange.Start > 0 && f.SourceRange.Start < len(input) {
			if !utf8.RuneStart(input[f.SourceRange.Start]) {
				return nil, fmt.Errorf("source range start at %d is not a valid UTF-8 boundary", f.SourceRange.Start)
			}
		}
		if f.SourceRange.End > 0 && f.SourceRange.End < len(input) {
			if !utf8.RuneStart(input[f.SourceRange.End]) {
				return nil, fmt.Errorf("source range end at %d is not a valid UTF-8 boundary", f.SourceRange.End)
			}
		}
		if len(f.RuleIDs) == 0 {
			return nil, errors.New("missing rule ids")
		}
		for _, id := range f.RuleIDs {
			if active != nil {
				if _, ok := active[id]; !ok {
					return nil, fmt.Errorf("inactive rule id: %s", id)
				}
			} else if !strings.HasPrefix(id, "CORE.") && !strings.HasPrefix(id, "PROFILE_ID.") {
				return nil, fmt.Errorf("unknown rule id: %s", id)
			}
		}
		combined := f.Replacement + " " + f.Reason
		lower := strings.ToLower(combined)
		if strings.Contains(lower, "certif") || strings.Contains(lower, "compliance") || strings.Contains(lower, "guarantee") {
			return nil, errors.New("advisor response changes claims")
		}
		if f.Confidence < 0 || f.Confidence > 1 {
			return nil, errors.New("invalid confidence")
		}
		path := ""
		repl := f.Replacement
		out = append(out, report.Finding{
			RuleID:      f.RuleIDs[0],
			Checker:     "llm-advisor",
			Enforcement: "advisory",
			Severity:    "info",
			Message:     f.Reason,
			Suggestion:  &repl,
			Confidence:  f.Confidence,
			Path:        &path,
			Range:       &report.FindingRange{StartByte: f.SourceRange.Start, EndByte: f.SourceRange.End},
		})
	}
	return out, nil
}
