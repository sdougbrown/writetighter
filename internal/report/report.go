package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Report struct {
	SchemaVersion int          `json:"schema_version"`
	ToolVersion   string       `json:"tool_version"`
	Source        SourceInfo   `json:"source"`
	Profile       ProfileInfo  `json:"profile"`
	TermBase      TermBaseInfo `json:"term_base"`
	Status        string       `json:"status"`
	Claims        ClaimsInfo   `json:"claims"`
	Coverage      CoverageInfo `json:"coverage"`
	Findings      []Finding    `json:"findings"`
}

type SourceInfo struct {
	Kind string  `json:"kind"`
	Path *string `json:"path"`
}

type ProfileInfo struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type TermBaseInfo struct {
	SHA256 string `json:"sha256"`
}

type ClaimsInfo struct {
	Standard      *string `json:"standard"`
	Issue         *string `json:"issue"`
	Certification string  `json:"certification"`
}

type CoverageInfo struct {
	Rules []RuleCoverage `json:"rules"`
	LLM   string         `json:"llm"`
}

type RuleCoverage struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	State   string `json:"state"`
}

type Finding struct {
	RuleID         string        `json:"rule_id"`
	RuleVersion    int           `json:"rule_version"`
	Checker        string        `json:"checker"`
	CheckerVersion int           `json:"checker_version"`
	Enforcement    string        `json:"enforcement"`
	Severity       string        `json:"severity"`
	Path           *string       `json:"path"`
	Range          *FindingRange `json:"range"`
	Evidence       string        `json:"evidence"`
	Message        string        `json:"message"`
	Suggestion     *string       `json:"suggestion"`
	Confidence     float64       `json:"confidence"`
}

type FindingRange struct {
	StartByte   int `json:"start_byte"`
	EndByte     int `json:"end_byte"`
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

func RenderJSON(report *Report) (string, error) {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

func RenderHuman(report *Report) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", report.Status)
	for _, f := range report.Findings {
		fmt.Fprintf(&b, "%s %s %s\n", f.Severity, f.RuleID, f.Message)
	}
	return b.String(), nil
}

func RenderAgent(report *Report) (string, error) {
	var b strings.Builder
	for _, f := range report.Findings {
		path := "null"
		if f.Path != nil {
			path = *f.Path
		}
		fmt.Fprintf(&b, "check: %s %s path:%s\n", strings.ToUpper(f.Severity), f.RuleID, path)
	}
	return b.String(), nil
}
