package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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
	LLMProvider     string
	LLMAPIKeyEnv    string
	LLMTimeout      time.Duration
	FailOn          string
}

type RunFunc func() error

type App struct{}

func New() *App { return &App{} }

func (a *App) RunCheck(params CheckParams) error {
	if !validKind(params.Kind) || !validFormat(params.Format) || !validFailOn(params.FailOn) {
		return fmt.Errorf("invalid check option")
	}
	// Explicit configuration errors are fatal; absent discovered files are benign.
	userCfg, err := config.LoadUserConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		userCfg = nil
	}

	// Load project config
	var projCfg *config.ProjectConfig
	if params.ConfigPath != "" {
		var err error
		projCfg, err = config.LoadProjectConfig(params.ConfigPath)
		if err != nil {
			return err
		}
	} else {
		var err error
		projCfg, _, err = config.DiscoverProjectConfig()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	// Merge
	merged, err := config.MergeConfigs(projCfg, userCfg)
	if err != nil {
		return err
	}
	// Profile resolution intentionally precedes source reading.
	profileSpec := params.Profile
	if profileSpec == "" && projCfg != nil && projCfg.Profile.ID != "" {
		profileSpec = projCfg.Profile.ID + "@" + projCfg.Profile.Version
	}
	if profileSpec == "" && userCfg != nil && userCfg.Profile.ID != "" {
		profileSpec = userCfg.Profile.ID + "@" + userCfg.Profile.Version
	}
	r, err := profile.Resolve(profileSpec)
	if err != nil {
		return err
	}

	// Extract terms
	var terms []config.TermEntry
	if merged != nil && merged.Project != nil {
		terms = merged.Project.Terms
	}

	// LLM fallbacks from user config
	if merged != nil && merged.User != nil {
		uc := merged.User.LLM
		if params.LLMProvider == "" {
			params.LLMProvider = uc.Provider
		}
		if params.LLMBaseURL == "" && uc.BaseURL != "" {
			params.LLMBaseURL = uc.BaseURL
		}
		if params.LLMModel == "" && uc.Model != "" {
			params.LLMModel = uc.Model
		}
		if params.LLMResponseMode == "" && uc.ResponseMode != "" {
			params.LLMResponseMode = uc.ResponseMode
		}
		if params.LLMAPIKeyEnv == "" {
			params.LLMAPIKeyEnv = uc.APIKeyEnv
		}
		if params.LLMTimeout == 0 && uc.Timeout != "" {
			d, e := time.ParseDuration(uc.Timeout)
			if e != nil {
				return fmt.Errorf("invalid llm timeout: %w", e)
			}
			params.LLMTimeout = d
		}
	}
	if params.LLM {
		if params.LLMProvider != "" && params.LLMProvider != "openai-compatible" {
			return fmt.Errorf("unsupported llm provider %q", params.LLMProvider)
		}
		if params.LLMResponseMode == "" {
			params.LLMResponseMode = "auto"
		}
		if !validResponseMode(params.LLMResponseMode) {
			return fmt.Errorf("invalid llm response mode %q", params.LLMResponseMode)
		}
		if params.LLMTimeout == 0 {
			params.LLMTimeout = llm.DefaultTimeout
		}
		if params.LLMTimeout <= 0 || params.LLMModel == "" {
			return errors.New("invalid llm configuration")
		}
		if _, err := llm.NewClient(llm.Config{BaseURL: params.LLMBaseURL, Model: params.LLMModel, APIKeyEnv: params.LLMAPIKeyEnv, Timeout: params.LLMTimeout, ResponseMode: params.LLMResponseMode}); err != nil {
			return err
		}
	}
	docs, err := document.CollectInputs(params.Paths, params.Stdin)
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
		fmt.Fprintf(os.Stderr, "llm host: %s\n", llm.Host(params.LLMBaseURL))
		advisorConfig := llm.Config{BaseURL: params.LLMBaseURL, Model: params.LLMModel, APIKeyEnv: params.LLMAPIKeyEnv, ResponseMode: params.LLMResponseMode, Timeout: params.LLMTimeout}
		var advisory []report.Finding
		requests := 0
		for _, doc := range docs {
			var static []report.Finding
			for _, f := range findings {
				if f.Path == nil || *f.Path == doc.Source {
					static = append(static, f)
				}
			}
			if len(static) == 0 {
				continue
			}
			requests++
			more, callErr := llm.Advisor(context.Background(), advisorConfig, doc, r, static, terms)
			if callErr != nil {
				err = callErr
				break
			}
			advisory = append(advisory, more...)
		}
		if requests == 0 {
			llmState = "skipped"
		} else if err != nil {
			if params.RequireLLM {
				llmState = "failed"
			} else {
				fmt.Fprintf(os.Stderr, "llm advisor failed: %v\n", err)
				llmState = "failed"
			}
			err = nil // clear so report rendering proceeds
		} else {
			findings = append(findings, advisory...)
			llmState = "success"
		}
	}
	var sourcePath *string
	if !params.Stdin && len(params.Paths) > 0 {
		sourcePath = &params.Paths[0]
	}
	fail := false
	if params.FailOn == "error" {
		for _, f := range findings {
			if f.Severity == "error" {
				fail = true
			}
		}
	} else if params.FailOn == "warning" {
		for _, f := range findings {
			if f.Severity == "warning" || f.Severity == "error" {
				fail = true
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
	// Required LLM failure/skip must return 3 with the completed report preserved.
	if params.RequireLLM && (llmState == "failed" || llmState == "skipped") {
		return ErrRequireLLM
	}
	if fail {
		return ErrFailThreshold
	}
	return nil
}

func validKind(v string) bool   { return v == "description" || v == "procedure" || v == "pr" }
func validFormat(v string) bool { return v == "human" || v == "json" || v == "agent" }
func validFailOn(v string) bool { return v == "none" || v == "warning" || v == "error" }
func validResponseMode(v string) bool {
	return v == "auto" || v == "json_schema" || v == "json_object" || v == "prompt_json"
}

func (a *App) RunExplainWithOptions(term, profileSpec, format string) error {
	if format != "human" && format != "json" {
		return fmt.Errorf("unsupported format %q", format)
	}
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

func (a *App) RunProfileList(format string) error {
	embedded, err := profile.LoadEmbedded()
	if err != nil {
		return err
	}
	installed, err := profile.ListInstalled()
	if err != nil {
		return err
	}
	type listed struct{ ID, Version, SHA256, Source string }
	all := []listed{{string(embedded.ID), string(embedded.Version), embedded.SHA256, "embedded"}}
	for _, r := range installed {
		if r.ID == embedded.ID && r.Version == embedded.Version {
			if r.SHA256 != embedded.SHA256 {
				return profileConflictErrForApp(r)
			}
			continue
		}
		all = append(all, listed{string(r.ID), string(r.Version), r.SHA256, "installed"})
	}
	if format == "json" {
		profiles := make([]map[string]any, 0, len(all))
		for _, item := range all {
			profiles = append(profiles, map[string]any{"id": item.ID, "version": item.Version, "sha256": item.SHA256, "source": item.Source})
		}
		data := map[string]any{"profiles": profiles}
		return json.NewEncoder(os.Stdout).Encode(data)
	}
	for _, item := range all {
		fmt.Fprintf(os.Stdout, "%s  %s@%s  sha256:%s\n", item.Source, item.ID, item.Version, item.SHA256)
	}
	return nil
}

func profileConflictErrForApp(r *profile.Resolution) error {
	return fmt.Errorf("installed profile conflicts with embedded %s@%s", r.ID, r.Version)
}

func (a *App) RunProfileVerify(spec, format string) error {
	if info, err := os.Stat(spec); err == nil && info.IsDir() {
		result := profile.VerifyBundle(spec)
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(result)
		}
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
		if format == "json" {
			data := map[string]any{"id": string(resolution.ID), "version": string(resolution.Version), "sha256": resolution.SHA256, "valid": true}
			return json.NewEncoder(os.Stdout).Encode(data)
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
