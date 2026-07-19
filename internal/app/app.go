package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sdougbrown/writetighter/internal/check"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/llm"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

var (
	Version          = "0.1.0"
	Commit           = "unknown"
	ErrFailThreshold = errors.New("fail threshold reached")
	ErrRequireLLM    = errors.New("required llm failed")
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
	// Load user config
	userCfg, _ := config.LoadUserConfig()

	// Load project config
	var projCfg *config.ProjectConfig
	if params.ConfigPath != "" {
		projCfg, _ = config.LoadProjectConfig(params.ConfigPath)
	} else {
		projCfg, _, _ = config.DiscoverProjectConfig()
	}

	// Merge
	merged, _ := config.MergeConfigs(projCfg, userCfg)

	// Extract terms
	var terms []config.TermEntry
	if merged != nil && merged.Project != nil {
		terms = merged.Project.Terms
	}

	// LLM fallbacks from user config
	if merged != nil && merged.User != nil {
		uc := merged.User.LLM
		if params.LLMBaseURL == "" && uc.BaseURL != "" {
			params.LLMBaseURL = uc.BaseURL
		}
		if params.LLMModel == "" && uc.Model != "" {
			params.LLMModel = uc.Model
		}
		if params.LLMResponseMode == "" && uc.ResponseMode != "" {
			params.LLMResponseMode = uc.ResponseMode
		}
	}

	docs, err := document.CollectInputs(params.Paths, params.Stdin)
	if err != nil {
		return err
	}
	r, err := profile.Resolve(params.Profile)
	if err != nil {
		return err
	}
	if len(terms) > 0 && r != nil && r.Dict != nil {
		if verr := profile.ValidateAgainstProfile(terms, r.Dict); verr != nil {
			return verr
		}
	}
	enabled := check.Enabled(r)
	findings := []report.Finding{}

	coverage := make([]report.RuleCoverage, 0, len(r.Rules.Rules)+len(check.All()))

	// Build coverage from profile rules
	profileRuleIDs := map[string]bool{}
	for _, rule := range r.Rules.Rules {
		profileRuleIDs[rule.ID] = true
		c := check.Get(rule.ID)
		state := rule.Enforcement
		if state == "" {
			state = "disabled"
		}
		if c == nil {
			state = "not-implemented"
		}
		coverage = append(coverage, report.RuleCoverage{ID: rule.ID, Version: rule.Version, State: state})
	}

	// Add registered checkers not in profile as disabled
	for _, c := range check.All() {
		if !profileRuleIDs[c.ID()] {
			coverage = append(coverage, report.RuleCoverage{ID: c.ID(), Version: c.Version(), State: "disabled"})
		}
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
	llmState := "not-requested"
	if params.LLM {
		llmState = "requested"
	}
	if params.LLM {
		fmt.Fprintf(os.Stderr, "llm host: %s\n", llm.Host(params.LLMBaseURL))
		advisorConfig := llm.Config{BaseURL: params.LLMBaseURL, Model: params.LLMModel, ResponseMode: params.LLMResponseMode, Timeout: llm.DefaultTimeout}
		more, err := llm.Advisor(context.Background(), advisorConfig, docs[0], r, findings)
		if err != nil && !params.RequireLLM {
			fmt.Fprintf(os.Stderr, "llm advisor failed: %v\n", err)
			llmState = "failed"
		} else if err != nil {
			return err
		} else {
			findings = append(findings, more...)
			llmState = "success"
		}
	}
	var sourcePath *string
	if !params.Stdin && len(params.Paths) > 0 {
		sourcePath = &params.Paths[0]
	}
	if params.FailOn == "error" {
		for _, f := range findings {
			if f.Severity == "error" {
				return ErrFailThreshold
			}
		}
	} else if params.FailOn == "warning" {
		for _, f := range findings {
			if f.Severity == "warning" || f.Severity == "error" {
				return ErrFailThreshold
			}
		}
	}
	claims := report.ClaimsInfo{}
	if r.Manifest != nil {
		claims = report.ClaimsInfo{
			Standard:      r.Manifest.Claims.Standard,
			Issue:         r.Manifest.Claims.Issue,
			Certification: r.Manifest.Claims.Certification,
		}
	}
	termBaseHash := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if len(terms) > 0 {
		tbData, _ := json.Marshal(terms)
		termBaseHash = "sha256:" + profile.SHA256Bytes(tbData)
	}
	reportModel := &report.Report{
		SchemaVersion: 1,
		ToolVersion:   Version,
		Source:        report.SourceInfo{Kind: params.Kind, Path: sourcePath},
		Profile:       report.ProfileInfo{ID: string(r.ID), Version: string(r.Version), SHA256: r.SHA256},
		TermBase:      report.TermBaseInfo{SHA256: termBaseHash},
		Status:        "checked",
		Claims:        claims,
		Coverage:      report.CoverageInfo{Rules: coverage, LLM: llmState},
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
