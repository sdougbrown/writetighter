package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/sdougbrown/writetighter/internal/document"
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
	var sourcePath *string
	if !params.Stdin && len(params.Paths) > 0 {
		sourcePath = &params.Paths[0]
	}
	reportModel := &report.Report{
		SchemaVersion: 1,
		ToolVersion:   "0.1.0",
		Source:        report.SourceInfo{Kind: params.Kind, Path: sourcePath},
		Profile:       report.ProfileInfo{ID: "software-docs-en", Version: "0.1.0", SHA256: "placeholder"},
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

func (a *App) RunExplainWithOptions(_ string, _ string, _ string) error {
	return errors.New("not implemented")
}

func (a *App) RunProfileInstall(_ string) error { return errors.New("not implemented") }

func (a *App) RunProfileList() error { return errors.New("not implemented") }

func (a *App) RunProfileVerify(_ string) error { return errors.New("not implemented") }

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
