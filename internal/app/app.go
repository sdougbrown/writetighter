package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sdougbrown/writetighter/internal/check"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

type CheckParams struct {
	Paths           []string
	Stdin           bool
	Kind            string
	Profile         string
	ConfigPath      string
	Format          string
	LLM             bool
	RequireLLM      bool
	LLMBaseURL      string
	LLMModel        string
	LLMResponseMode string
	FailOn          string
}

type RunFunc func() error

type App struct{}

func New() *App { return &App{} }

func (a *App) RunCheck(params CheckParams) error {
	docs, err := document.CollectInputs(params.Paths, params.Stdin)
	if err != nil {
		return err
	}
	r, err := profile.Resolve(params.Profile)
	if err != nil {
		return err
	}
	var terms []config.TermEntry
	enabled := check.Enabled(r)
	findings := []report.Finding{}
	profileRuleEnabled := map[string]bool{}
	for _, rule := range r.Rules.Rules {
		profileRuleEnabled[rule.ID] = rule.Enabled
	}

	allCheckers := check.All()
	coverage := make([]report.RuleCoverage, 0, len(allCheckers))
	for _, c := range allCheckers {
		state := "disabled"
		if profileRuleEnabled[c.ID()] {
			state = "enabled"
		}
		coverage = append(coverage, report.RuleCoverage{ID: c.ID(), Version: c.Version(), State: state})
	}
	for _, doc := range docs {
		if params.Kind != "" {
			doc.Kind = params.Kind
		}
		ctx := &check.RunContext{Document: doc, Profile: r, Terms: terms}
		for _, c := range enabled {
			more, err := c.Run(ctx)
			if err != nil {
				return err
			}
			findings = append(findings, more...)
		}
	}
	var sourcePath *string
	if !params.Stdin && len(params.Paths) > 0 {
		sourcePath = &params.Paths[0]
	}
	reportModel := &report.Report{
		SchemaVersion: 1,
		ToolVersion:   "0.1.0",
		Source:        report.SourceInfo{Kind: params.Kind, Path: sourcePath},
		Profile:       report.ProfileInfo{ID: string(r.ID), Version: string(r.Version), SHA256: r.SHA256},
		TermBase:      report.TermBaseInfo{SHA256: "placeholder"},
		Status:        "checked",
		Claims:        report.ClaimsInfo{Certification: "unknown"},
		Coverage:      report.CoverageInfo{Rules: coverage, LLM: "not-requested"},
		Findings:      findings,
	}
	formatted, err := renderReport(reportModel, params.Format)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(os.Stdout, formatted)
	return nil
}

func (a *App) RunExplain(_ string) error { return errors.New("not implemented") }

func (a *App) RunExplainWithOptions(term, profileSpec, format string) error {
	_, err := profile.Resolve(profileSpec)
	if err != nil {
		return err
	}
	if c := check.Get(term); c != nil {
		switch format {
		case "json":
			data := map[string]any{"id": c.ID(), "version": c.Version()}
			return json.NewEncoder(os.Stdout).Encode(data)
		default:
			_, err = fmt.Fprintf(os.Stdout, "%s v%d\n", c.ID(), c.Version())
			return err
		}
	}
	return fmt.Errorf("rule not found: %s", term)
}

func (a *App) RunProfileInstall(spec string) error {
	_, err := profile.InstallBundle(spec)
	return err
}

func (a *App) RunProfileList() error {
	embedded, err := profile.LoadEmbedded()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "embedded  %s@%s  sha256:%s\n", embedded.ID, embedded.Version, embedded.SHA256)
	return nil
}

func (a *App) RunProfileVerify(spec string) error {
	if info, err := os.Stat(spec); err == nil && info.IsDir() {
		result := profile.VerifyBundle(spec)
		if !result.Valid {
			return fmt.Errorf("profile verification failed")
		}
		return nil
	}
	if strings.Contains(spec, "@") {
		resolution, err := profile.Resolve(spec)
		if err != nil {
			return fmt.Errorf("profile not found: %s", spec)
		}
		fmt.Fprintf(os.Stderr, "Profile %s@%s resolved (embedded): SHA256=%s\n", resolution.ID, resolution.Version, resolution.SHA256)
		return nil
	}
	return fmt.Errorf("profile not found: %s", spec)
}

func renderReport(r *report.Report, format string) (string, error) {
	switch format {
	case "json", "":
		return report.RenderJSON(r)
	case "human":
		return report.RenderHuman(r)
	case "agent":
		return report.RenderAgent(r)
	default:
		return "", errors.New("unknown format")
	}
}
