package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

func ValidateAdvisorResponse(raw []byte, input string) ([]report.Finding, error) {
	if len(raw) > MaxOutputChars {
		return nil, errors.New("llm response too large")
	}
	var resp AdvisorResponse
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&resp); err != nil {
		return nil, err
	}
	if len(resp.Findings) > MaxSuggestions {
		return nil, errors.New("too many llm suggestions")
	}
	var out []report.Finding
	for _, f := range resp.Findings {
		if f.SourceRange.Start < 0 || f.SourceRange.End < f.SourceRange.Start || f.SourceRange.End > len(input) {
			return nil, fmt.Errorf("invalid source range")
		}
		if len(f.RuleIDs) == 0 {
			return nil, errors.New("missing rule ids")
		}
		for _, id := range f.RuleIDs {
			if !strings.HasPrefix(id, "CORE.") && !strings.HasPrefix(id, "PROFILE_ID.") {
				return nil, fmt.Errorf("unknown rule id: %s", id)
			}
		}
		lower := strings.ToLower(f.Replacement)
		if strings.Contains(lower, "certif") || strings.Contains(lower, "compliance") || strings.Contains(lower, "guarantee") {
			return nil, errors.New("replacement changes claims")
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
