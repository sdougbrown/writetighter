package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sdougbrown/writetighter/internal/check"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/llm"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

const (
	defaultRevisionChunkBytes = 20 * 1024
	defaultMaxModelRequests   = 32
)

var (
	Version              = "0.1.0"
	Commit               = "unknown"
	ErrFailThreshold     = errors.New("fail threshold reached")
	ErrLLMConfigRequired = errors.New("model configuration required")
	ErrReviseFailed      = errors.New("revise failed")
)

type LintParams struct {
	Paths      []string
	Stdin      bool
	Text       *string
	Kind       string
	Profile    string
	ConfigPath string
	Format     string
	FailOn     string
}

// ReviseParams holds parameters for the `writetighter revise` command.
type ReviseParams struct {
	Paths          []string
	Stdin          bool
	Text           *string
	Kind           string
	Profile        string
	ConfigPath     string
	Format         string
	ReferencePaths []string
}

// PromptParams selects exported revision guidance without model access.
type PromptParams struct {
	Kind   string
	Format string
}

type RunFunc func() error

type App struct{}

func New() *App { return &App{} }

func (a *App) RunLint(params LintParams) error {
	if !validKind(params.Kind) || !validFormat(params.Format) || !validFailOn(params.FailOn) {
		return fmt.Errorf("invalid lint option")
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

	docs, err := collectInputs(params.Paths, params.Stdin, params.Text, params.Kind)
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
	var sourcePath *string
	if params.Text != nil {
		textSource := "<text>"
		sourcePath = &textSource
	} else if !params.Stdin && len(params.Paths) > 0 {
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
		Status:        "linted",
		Claims:        claims,
		Coverage:      report.CoverageInfo{Rules: coverage},
		Findings:      findings,
	}
	formatted, err := renderReport(reportModel, params.Format)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(os.Stdout, formatted)
	if fail {
		return ErrFailThreshold
	}
	return nil
}

func collectInputs(paths []string, stdin bool, text *string, kind string) ([]*document.Document, error) {
	selected := 0
	if len(paths) > 0 {
		selected++
	}
	if stdin {
		selected++
	}
	if text != nil {
		selected++
	}
	if selected == 0 {
		return nil, errors.New("no input specified")
	}
	if selected > 1 {
		return nil, errors.New("paths, --stdin, and --text are mutually exclusive")
	}
	if text != nil {
		doc, err := document.FromText(*text, kind)
		if err != nil {
			return nil, err
		}
		return []*document.Document{doc}, nil
	}
	return document.CollectInputs(paths, stdin)
}

func validKind(v string) bool   { return guidance.ValidKind(v) }
func validFormat(v string) bool { return v == "human" || v == "json" || v == "agent" }
func validFailOn(v string) bool { return v == "none" || v == "warning" || v == "error" }
func validResponseMode(v string) bool {
	return v == "auto" || v == "json_schema" || v == "json_object" || v == "prompt_json"
}

// RunPrompt prints the same core and kind-specific guidance used by revise.
// It does not load user configuration, resolve a profile, read input, or call a model.
func (a *App) RunPrompt(params PromptParams) error {
	if params.Kind == "" {
		params.Kind = guidance.KindDescription
	}
	if params.Format == "" {
		params.Format = "human"
	}
	if params.Format != "human" && params.Format != "json" {
		return fmt.Errorf("invalid prompt format %q", params.Format)
	}
	rubric, err := guidance.ForKind(params.Kind)
	if err != nil {
		return err
	}
	if params.Format == "json" {
		return json.NewEncoder(os.Stdout).Encode(rubric)
	}
	var b strings.Builder
	b.WriteString("You are a technical writing reviewer. Revise prose using the following guidance.\n")
	fmt.Fprintf(&b, "Document kind: %s\n\n", rubric.Kind)
	b.WriteString("Revision principles:\n")
	for _, principle := range rubric.Principles {
		fmt.Fprintf(&b, "- %s: %s\n", principle.ID, principle.Direction)
	}
	b.WriteString("\nCore directions:\n")
	for _, direction := range rubric.CoreDirections {
		fmt.Fprintf(&b, "- %s\n", direction)
	}
	b.WriteString("\nDocument-kind directions:\n")
	for _, direction := range rubric.KindDirections {
		fmt.Fprintf(&b, "- %s\n", direction)
	}
	_, err = fmt.Fprint(os.Stdout, b.String())
	return err
}

// RunRevise runs contextual model revision. It reads LLM config from the
// user config [llm] section and returns a structured ReviseResponse.
func (a *App) RunRevise(params ReviseParams) error {
	if params.Kind == "" {
		params.Kind = "description"
	}
	if !validKind(params.Kind) {
		return fmt.Errorf("invalid document kind %q", params.Kind)
	}
	selectedInputs := 0
	if len(params.Paths) > 0 {
		selectedInputs++
	}
	if params.Stdin {
		selectedInputs++
	}
	if params.Text != nil {
		selectedInputs++
	}
	if selectedInputs == 0 {
		return fmt.Errorf("no input specified")
	}
	if selectedInputs > 1 {
		return fmt.Errorf("paths, --stdin, and --text are mutually exclusive")
	}
	if params.Format == "" {
		params.Format = "json" // revise defaults to JSON output
	}
	if params.Format != "json" && params.Format != "human" {
		return fmt.Errorf("invalid format %q for revise", params.Format)
	}

	// Load user config (required for model settings).
	userCfg, err := config.LoadUserConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: user config could not be loaded", ErrLLMConfigRequired)
	}
	if errors.Is(err, os.ErrNotExist) {
		userCfg = nil
	}

	// Load project config.
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

	// Merge configs.
	merged, err := config.MergeConfigs(projCfg, userCfg)
	if err != nil {
		return err
	}

	// Resolve profile.
	profileSpec := params.Profile
	if profileSpec == "" && projCfg != nil && projCfg.Profile.ID != "" {
		profileSpec = projCfg.Profile.ID + "@" + projCfg.Profile.Version
	}
	if profileSpec == "" && userCfg != nil && userCfg.Profile.ID != "" {
		profileSpec = userCfg.Profile.ID + "@" + userCfg.Profile.Version
	}
	res, err := profile.Resolve(profileSpec)
	if err != nil {
		return err
	}

	// Extract terms.
	var terms []config.TermEntry
	if merged != nil && merged.Project != nil {
		terms = merged.Project.Terms
	}

	// Read LLM config from user config. Require usable configuration.
	if merged == nil || merged.User == nil || merged.User.LLM.Model == "" {
		return fmt.Errorf("%w: revise requires an [llm] model", ErrLLMConfigRequired)
	}
	uc := merged.User.LLM

	llmProvider := uc.Provider
	llmBaseURL := uc.BaseURL
	llmModel := uc.Model
	llmAPIKey := uc.APIKey
	llmAPIKeyEnv := uc.APIKeyEnv
	llmResponseMode := uc.ResponseMode
	llmTimeout := llm.DefaultTimeout
	maxRequests := uc.MaxRequests
	if maxRequests == 0 {
		maxRequests = defaultMaxModelRequests
	}
	if uc.Timeout != "" {
		d, e := time.ParseDuration(uc.Timeout)
		if e != nil {
			return fmt.Errorf("%w: invalid llm timeout: %v", ErrLLMConfigRequired, e)
		}
		llmTimeout = d
	}

	if llmProvider == "" {
		llmProvider = "openai-compatible"
	}
	if llmProvider != "openai-compatible" {
		return fmt.Errorf("%w: unsupported llm provider %q", ErrLLMConfigRequired, llmProvider)
	}
	if llmResponseMode == "" {
		llmResponseMode = "auto"
	}
	if !validResponseMode(llmResponseMode) {
		return fmt.Errorf("%w: invalid llm response mode %q", ErrLLMConfigRequired, llmResponseMode)
	}
	if llmModel == "" || llmBaseURL == "" {
		return fmt.Errorf("%w: revise requires llm model and base_url", ErrLLMConfigRequired)
	}
	if maxRequests < 1 || maxRequests > 1024 {
		return fmt.Errorf("%w: max_requests must be between 1 and 1024", ErrLLMConfigRequired)
	}
	if llmAPIKey == "" && llmAPIKeyEnv != "" && os.Getenv(llmAPIKeyEnv) == "" {
		return fmt.Errorf("%w: api_key_env %q is configured but the environment variable is unset", ErrLLMConfigRequired, llmAPIKeyEnv)
	}

	llmCfg := llm.Config{
		BaseURL:      llmBaseURL,
		Model:        llmModel,
		APIKey:       llmAPIKey,
		APIKeyEnv:    llmAPIKeyEnv,
		Timeout:      llmTimeout,
		ResponseMode: llmResponseMode,
	}

	// Validate LLM config before reading input.
	if _, err := llm.NewClient(llmCfg); err != nil {
		return fmt.Errorf("%w: %v", ErrLLMConfigRequired, err)
	}

	// Collect input documents.
	docs, err := collectInputs(params.Paths, params.Stdin, params.Text, params.Kind)
	if err != nil {
		return err
	}

	// Validate terms against profile.
	if len(terms) > 0 && res != nil && res.Dict != nil {
		if verr := profile.ValidateAgainstProfile(terms, res.Dict); verr != nil {
			return verr
		}
	}

	// Run static lint findings (for context, not prerequisites).
	enabled := check.Enabled(res)
	findings := []report.Finding{}
	for _, doc := range docs {
		if params.Kind != "" {
			doc.Kind = params.Kind
		}
		ctx := &check.RunContext{Document: doc, Profile: res, Terms: terms}
		for _, c := range enabled {
			more, err := c.Run(ctx)
			if err != nil {
				return err
			}
			findings = append(findings, more...)
		}
	}

	type revisionPlan struct {
		doc    *document.Document
		chunks []document.ChunkRange
	}
	plans := make([]revisionPlan, 0, len(docs))
	totalRequests := 0
	for _, doc := range docs {
		chunks := document.ChunkRanges(doc, defaultRevisionChunkBytes)
		totalRequests += len(chunks)
		plans = append(plans, revisionPlan{doc: doc, chunks: chunks})
	}
	if totalRequests > maxRequests {
		return fmt.Errorf("revision requires %d model requests, exceeding configured max_requests=%d", totalRequests, maxRequests)
	}

	// Process chunks sequentially to avoid overloading local endpoints and keep
	// output deterministic. Chunk ranges remain relative to the original file.
	allRevisions := make([]report.RevisionItem, 0)
	revisionErrors := make([]report.RevisionError, 0)
	analysis := make([]report.SourceAnalysis, 0, len(plans))
	discardedRewrites := 0
	succeededRequests := 0
	requestsMade := 0
	remainingPrimaryRequests := totalRequests
	for _, plan := range plans {
		docFindings := make([]report.Finding, 0)
		for _, f := range findings {
			if f.Path == nil || *f.Path == plan.doc.Source {
				docFindings = append(docFindings, f)
			}
		}
		coverage := report.SourceAnalysis{SourcePath: plan.doc.Source, InputBytes: len(plan.doc.Content), Chunks: len(plan.chunks)}
		if plan.doc.Format == document.FormatHTML {
			coverage.SourceFormat = "html"
			coverage.RangeBasis = "visible_text"
		}
		for _, chunk := range plan.chunks {
			requestsMade++
			remainingPrimaryRequests--
			coverage.ModelRequests++
			revResp, callErr := llm.ReviseChunk(context.Background(), llmCfg, plan.doc, res, docFindings, terms, chunk.StartByte, chunk.EndByte)
			if callErr != nil && errors.Is(callErr, llm.ErrInvalidModelResponse) && (llmResponseMode == "json_object" || llmResponseMode == "json_schema") && requestsMade+remainingPrimaryRequests < maxRequests {
				fallbackCfg := llmCfg
				fallbackCfg.ResponseMode = "prompt_json"
				requestsMade++
				coverage.ModelRequests++
				fallbackResp, fallbackErr := llm.ReviseChunk(context.Background(), fallbackCfg, plan.doc, res, docFindings, terms, chunk.StartByte, chunk.EndByte)
				if fallbackErr == nil {
					revResp, callErr = fallbackResp, nil
				} else {
					callErr = fmt.Errorf("structured response failed: %v; prompt_json fallback failed: %v", callErr, fallbackErr)
				}
			}
			if callErr != nil {
				revisionErrors = append(revisionErrors, report.RevisionError{
					SourcePath: plan.doc.Source,
					Message:    fmt.Sprintf("chunk [%d,%d): %v", chunk.StartByte, chunk.EndByte, callErr),
				})
				continue
			}
			succeededRequests++
			coverage.AnalyzedBytes += chunk.EndByte - chunk.StartByte
			discardedRewrites += revResp.DiscardedRewrites
			allRevisions = append(allRevisions, revResp.Revisions...)
		}
		coverage.Complete = coverage.AnalyzedBytes == len(plan.doc.AnalysisContent())
		analysis = append(analysis, coverage)
	}
	sort.SliceStable(allRevisions, func(i, j int) bool {
		if allRevisions[i].SourcePath != allRevisions[j].SourcePath {
			return allRevisions[i].SourcePath < allRevisions[j].SourcePath
		}
		return allRevisions[i].Range.StartByte < allRevisions[j].Range.StartByte
	})

	sources := make([]string, 0, len(docs))
	for _, doc := range docs {
		sources = append(sources, doc.Source)
	}
	status := "ok"
	if len(revisionErrors) > 0 {
		status = "partial"
		if succeededRequests == 0 {
			status = "failed"
		}
	}
	reviseResponse := &report.ReviseResponse{
		SchemaVersion:     1,
		ToolVersion:       Version,
		ProfileID:         string(res.ID),
		ProfileVersion:    string(res.Version),
		LLMModel:          llmModel,
		LLMProvider:       llmProvider,
		Sources:           sources,
		Analysis:          analysis,
		Status:            status,
		DiscardedRewrites: discardedRewrites,
		Errors:            revisionErrors,
		Revisions:         allRevisions,
	}

	var formatted string
	if params.Format == "human" {
		formatted, err = report.RenderReviseHuman(reviseResponse)
	} else {
		formatted, err = report.RenderReviseJSON(reviseResponse)
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(os.Stdout, formatted)
	if len(revisionErrors) > 0 {
		return fmt.Errorf("%w: %d document revision request(s) failed", ErrReviseFailed, len(revisionErrors))
	}
	return nil
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
