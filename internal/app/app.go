package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
	_, err := document.CollectInputs(params.Paths, params.Stdin)
	if err != nil {
		return err
	}
	r, err := profile.Resolve(params.Profile)
	if err != nil {
		return err
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
		Coverage:      report.CoverageInfo{Rules: []report.RuleCoverage{}, LLM: "not-requested"},
		Findings:      []report.Finding{},
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
	_, err = fmt.Fprintln(os.Stdout, term, format)
	return err
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
