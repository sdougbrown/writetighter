package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/check"
	"github.com/sdougbrown/writetighter/internal/codecomment"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/corpus"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/llm"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/reference"
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
	ErrRewriteFailed     = errors.New("rewrite failed")
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
	GitCompare string
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

	// Model overrides the configured model for this revise run. When empty,
	// the [llm] model from user config is used.
	Model string

	// CodeModel overrides the model used for code-aware comment revision.
	// When empty, llm.code_model from user config is used, falling back to
	// the main model.
	CodeModel string

	// ContextTokens overrides the model context window for this revise run.
	// When 0, the context window is auto-detected from the /v1/models endpoint
	// when needed (reference or code-comment revision). When auto-detection
	// is unavailable, revise falls back to legacy byte-budget chunking.
	ContextTokens int

	// OutputTokens overrides the max output tokens sent to the API. When 0,
	// a default is used when a context window is known. When no context
	// window is known, max_tokens is not sent to the API.
	OutputTokens int
}

// RewriteParams holds parameters for the `writetighter rewrite` command.
// Unlike ReviseParams, rewrite sends the full passage as one model request
// and returns the complete rewritten text, not surgical findings.
type RewriteParams struct {
	Paths      []string
	Stdin      bool
	Text       *string
	Kind       string
	Profile    string
	ConfigPath string
	Format     string

	// Model overrides the configured model for this rewrite run.
	Model string
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

	if params.GitCompare != "" {
		if params.Stdin || params.Text != nil {
			return fmt.Errorf("--git-compare is only valid with file paths, not --stdin or --text")
		}
		if len(params.Paths) == 0 {
			return fmt.Errorf("--git-compare requires file paths")
		}
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

	// When --git-compare is passed, auto-enable the corpus-novelty checker.
	// The checker abstains without comparison data, so enabling it here is safe.
	if params.GitCompare != "" {
		if c := check.Get("CORE.CORPUS_NOVELTY"); c != nil {
			alreadyEnabled := false
			for _, e := range enabled {
				if e.ID() == "CORE.CORPUS_NOVELTY" {
					alreadyEnabled = true
					break
				}
			}
			if !alreadyEnabled {
				enabled = append(enabled, c)
			}
		}
	}
	findings := []report.Finding{}

	// Build comparison data and change counts for --git-compare.
	var gitCompare *corpus.GitCompare
	if params.GitCompare != "" {
		repoRoot, err := corpus.FindRepoRoot(params.Paths[0])
		if err != nil {
			return fmt.Errorf("--git-compare %q: %w", params.GitCompare, err)
		}
		gitCompare, err = corpus.BuildGitCompare(repoRoot, params.GitCompare)
		if err != nil {
			return fmt.Errorf("--git-compare %q: %w", params.GitCompare, err)
		}
		// Compute change counts across all selected documents.
		gitCompare.ChangeTermCounts = make(map[string]int)
		gitCompare.ChangePhraseCounts = make(map[string]int)
		for _, doc := range docs {
			text := docAnalysisText(doc)
			tc, pc := corpus.CountTerms(text)
			for t, c := range tc {
				gitCompare.ChangeTermCounts[t] += c
			}
			for p, c := range pc {
				gitCompare.ChangePhraseCounts[p] += c
			}
		}
	}

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
		var more []report.Finding
		if usesCodeCommentCatalog(params.Kind, params.Stdin, params.Text, doc) {
			more, err = lintCodeCommentCatalog(doc, r, terms, enabled, gitCompare)
		} else {
			more, err = runDeterministicChecks(doc, r, terms, enabled, gitCompare)
		}
		if err != nil {
			return err
		}
		findings = append(findings, more...)
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

func runDeterministicChecks(doc *document.Document, res *profile.Resolution, terms []config.TermEntry, enabled []check.Checker, gitCompare *corpus.GitCompare) ([]report.Finding, error) {
	ctx := &check.RunContext{Document: doc, Profile: res, Terms: terms, GitCompare: gitCompare}
	var findings []report.Finding
	for _, checker := range enabled {
		more, err := checker.Run(ctx)
		if err != nil {
			return nil, err
		}
		findings = append(findings, more...)
	}
	return findings, nil
}

type commentLintProjection struct {
	startByte int
	endByte   int
	comment   codecomment.Comment
}

func lintCodeCommentCatalog(doc *document.Document, res *profile.Resolution, terms []config.TermEntry, enabled []check.Checker, gitCompare *corpus.GitCompare) ([]report.Finding, error) {
	language, ok := codecomment.DetectLanguage(doc.Source)
	if !ok {
		return nil, fmt.Errorf("code-comment lint does not support %q", doc.Source)
	}
	catalog, err := codecomment.Extract(doc.Source, language, []byte(doc.Content))
	if err != nil {
		return nil, fmt.Errorf("cataloging comments in %q: %w", doc.Source, err)
	}
	var analysis strings.Builder
	projections := make([]commentLintProjection, 0, len(catalog.Comments))
	for i, comment := range catalog.Comments {
		if i > 0 {
			analysis.WriteString("\n\n")
		}
		start := analysis.Len()
		analysis.WriteString(comment.Text)
		projections = append(projections, commentLintProjection{startByte: start, endByte: analysis.Len(), comment: comment})
	}
	commentDoc, err := document.FromPlainText(analysis.String(), doc.Source, doc.Kind)
	if err != nil {
		return nil, err
	}
	findings, err := runDeterministicChecks(commentDoc, res, terms, enabled, gitCompare)
	if err != nil {
		return nil, err
	}
	for i := range findings {
		findings[i], err = mapProjectedCommentFinding(findings[i], projections, doc.Content, doc.Source)
		if err != nil {
			return nil, err
		}
	}
	return findings, nil
}

func mapProjectedCommentFinding(finding report.Finding, projections []commentLintProjection, source, sourcePath string) (report.Finding, error) {
	finding.Path = &sourcePath
	if finding.Range == nil {
		return finding, nil
	}
	for _, projection := range projections {
		if finding.Range.StartByte < projection.startByte || finding.Range.EndByte > projection.endByte {
			continue
		}
		finding.Range.StartByte -= projection.startByte
		finding.Range.EndByte -= projection.startByte
		return mapCommentFinding(finding, projection.comment, source, sourcePath)
	}
	return report.Finding{}, fmt.Errorf("checker %s returned range [%d,%d) outside catalog comments", finding.Checker, finding.Range.StartByte, finding.Range.EndByte)
}

func mapCommentFinding(finding report.Finding, comment codecomment.Comment, source, sourcePath string) (report.Finding, error) {
	finding.Path = &sourcePath
	if finding.Range == nil {
		return finding, nil
	}
	start := finding.Range.StartByte
	end := finding.Range.EndByte
	if start < 0 || end < start || end > len(comment.Text) {
		return report.Finding{}, fmt.Errorf("checker %s returned range [%d,%d) outside comment %s", finding.Checker, start, end, comment.ID)
	}
	start += comment.Span.StartByte
	end += comment.Span.StartByte
	startLine, startColumn := sourceLineColumn(source, start)
	endLine, endColumn := sourceLineColumn(source, end)
	finding.Range = &report.FindingRange{
		StartByte:   start,
		EndByte:     end,
		StartLine:   startLine,
		StartColumn: startColumn,
		EndLine:     endLine,
		EndColumn:   endColumn,
	}
	return finding, nil
}

func sourceLineColumn(content string, byteOffset int) (line, column int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(content) {
		byteOffset = len(content)
	}
	line, column = 1, 1
	for current := 0; current < byteOffset; {
		r, size := utf8.DecodeRuneInString(content[current:])
		if r == '\n' {
			line++
			column = 1
		} else {
			column++
		}
		current += size
	}
	return line, column
}

// docAnalysisText returns the text that will be analyzed by checkers for a
// given document. For code-comment files, this is the extracted comment
// text. For prose files, it is the analysis content (HTML visible text or
// raw content).
func docAnalysisText(doc *document.Document) string {
	language, ok := codecomment.DetectLanguage(doc.Source)
	if ok {
		catalog, err := codecomment.Extract(doc.Source, language, []byte(doc.Content))
		if err != nil {
			return doc.AnalysisContent()
		}
		var parts []string
		for _, c := range catalog.Comments {
			parts = append(parts, c.Text)
		}
		return strings.Join(parts, " ")
	}
	return doc.AnalysisContent()
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

// RunRewrite runs whole-passage contextual rewrite. It sends the full input
// as one model request and returns the complete rewritten text, not surgical
// findings. Lint findings are collected first (deterministic, no model) and
// passed as context to the rewrite. If the model call fails, the response is
// empty, or protected-content validation fails, the original text is returned.
func (a *App) RunRewrite(params RewriteParams) error {
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
		params.Format = "text" // rewrite defaults to plain text output
	}
	if params.Format != "text" && params.Format != "json" && params.Format != "human" {
		return fmt.Errorf("invalid format %q for rewrite", params.Format)
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
		projCfg, err = config.LoadProjectConfig(params.ConfigPath)
		if err != nil {
			return err
		}
	} else {
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

	// Read LLM config from user config.
	if merged == nil || merged.User == nil || merged.User.LLM.Model == "" {
		return fmt.Errorf("%w: rewrite requires an [llm] model", ErrLLMConfigRequired)
	}
	uc := merged.User.LLM

	llmBaseURL := uc.BaseURL
	llmModel := uc.Model
	llmAPIKey := uc.APIKey
	llmAPIKeyEnv := uc.APIKeyEnv
	llmTimeout := llm.DefaultTimeout
	if uc.Timeout != "" {
		d, e := time.ParseDuration(uc.Timeout)
		if e != nil {
			return fmt.Errorf("%w: invalid llm timeout: %v", ErrLLMConfigRequired, e)
		}
		llmTimeout = d
	}

	// Apply runtime model override.
	if params.Model != "" {
		llmModel = params.Model
	}

	llmProvider := uc.Provider
	if llmProvider == "" {
		llmProvider = "openai-compatible"
	}
	if llmProvider != "openai-compatible" {
		return fmt.Errorf("%w: unsupported llm provider %q", ErrLLMConfigRequired, llmProvider)
	}
	if llmModel == "" || llmBaseURL == "" {
		return fmt.Errorf("%w: rewrite requires llm model and base_url", ErrLLMConfigRequired)
	}
	if llmAPIKey == "" && llmAPIKeyEnv != "" && os.Getenv(llmAPIKeyEnv) == "" {
		return fmt.Errorf("%w: api_key_env %q is configured but the environment variable is unset", ErrLLMConfigRequired, llmAPIKeyEnv)
	}

	llmCfg := llm.Config{
		BaseURL:   llmBaseURL,
		Model:     llmModel,
		APIKey:    llmAPIKey,
		APIKeyEnv: llmAPIKeyEnv,
		Timeout:   llmTimeout,
		// ResponseMode is intentionally not set — rewrite uses plain text
		// output, not structured JSON. The model returns the rewritten
		// passage as the assistant message content.
	}

	// Validate LLM config.
	if _, err := llm.NewClient(llmCfg); err != nil {
		return fmt.Errorf("%w: %v", ErrLLMConfigRequired, err)
	}

	// Collect input documents.
	docs, err := collectInputs(params.Paths, params.Stdin, params.Text, params.Kind)
	if err != nil {
		return err
	}

	for _, doc := range docs {
		if params.Kind != "" {
			doc.Kind = params.Kind
		}
	}

	// Validate terms against profile.
	if len(terms) > 0 && res != nil && res.Dict != nil {
		if verr := profile.ValidateAgainstProfile(terms, res.Dict); verr != nil {
			return verr
		}
	}

	// Run deterministic lint first — findings are passed to the model as context.
	enabled := check.Enabled(res)
	findings := []report.Finding{}
	for _, doc := range docs {
		ctx := &check.RunContext{Document: doc, Profile: res, Terms: terms}
		for _, c := range enabled {
			more, err := c.Run(ctx)
			if err != nil {
				return err
			}
			findings = append(findings, more...)
		}
	}

	// Process each document: one rewrite request per document.
	for _, doc := range docs {
		docFindings := make([]report.Finding, 0)
		for _, f := range findings {
			if f.Path == nil || *f.Path == doc.Source {
				docFindings = append(docFindings, f)
			}
		}

		result, rerr := llm.Rewrite(context.Background(), llmCfg, doc, res, docFindings, terms)
		if rerr != nil {
			// Model failure — return the original text and report the error.
			if errors.Is(rerr, llm.ErrRewriteEmpty) {
				// Empty output is a soft failure: return the original.
				result = &llm.RewriteResult{Text: doc.Content, Discarded: false, ModelUsed: llmModel}
			} else {
				return fmt.Errorf("%w: %s: %v", ErrRewriteFailed, doc.Source, rerr)
			}
		}

		resp := &report.RewriteResponse{
			SchemaVersion:  1,
			ToolVersion:    Version,
			Status:         "ok",
			ProfileID:      string(res.ID),
			ProfileVersion: string(res.Version),
			LLMModel:       result.ModelUsed,
			LLMProvider:    llmProvider,
			SourcePath:     doc.Source,
			InputBytes:     len(doc.Content),
			OutputBytes:    len(result.Text),
			RewrittenText:  result.Text,
			Discarded:      result.Discarded,
			LintFindings:   len(docFindings),
		}
		if result.Discarded {
			resp.Status = "discarded"
		}

		var formatted string
		var renderErr error
		switch params.Format {
		case "json":
			formatted, renderErr = report.RenderRewriteJSON(resp)
		case "human":
			formatted, renderErr = report.RenderRewriteHuman(resp)
		default: // "text"
			formatted = result.Text
			if !strings.HasSuffix(formatted, "\n") {
				formatted += "\n"
			}
		}
		if renderErr != nil {
			return renderErr
		}
		_, _ = fmt.Fprint(os.Stdout, formatted)
	}
	return nil
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

	// Apply runtime model override.
	if params.Model != "" {
		llmModel = params.Model
	}

	// Determine the code-comment model: explicit flag, then config, then main.
	codeModel := params.CodeModel
	if codeModel == "" {
		codeModel = uc.CodeModel
	}
	if codeModel == "" {
		codeModel = llmModel
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

	for _, doc := range docs {
		if params.Kind != "" {
			doc.Kind = params.Kind
		}
		if len(params.ReferencePaths) > 0 && usesCodeCommentProtocol(params, doc) {
			return errors.New("code-aware comment revision does not support --reference; omit references or use the legacy stdin/text path")
		}
	}

	// Validate terms against profile.
	if len(terms) > 0 && res != nil && res.Dict != nil {
		if verr := profile.ValidateAgainstProfile(terms, res.Dict); verr != nil {
			return verr
		}
	}

	// Run static lint findings only for prose-style revision. Code-aware review
	// supplies complete source as read-only context and must not present prose
	// findings over executable tokens as model evidence.
	enabled := check.Enabled(res)
	findings := []report.Finding{}
	for _, doc := range docs {
		if usesCodeCommentProtocol(params, doc) {
			continue
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

	// Collect reference material if paths provided.
	var refPack *reference.Pack
	if len(params.ReferencePaths) > 0 {
		sourcePaths := make([]string, len(docs))
		for i, doc := range docs {
			sourcePaths[i] = doc.Source
		}
		refPack, err = reference.Collect(params.ReferencePaths, sourcePaths)
		if err != nil {
			return fmt.Errorf("collecting references: %w", err)
		}
	}

	// Determine whether any document uses the code-aware comment protocol.
	// This affects which model needs context window resolution.
	hasCodeAware := false
	for _, doc := range docs {
		if usesCodeCommentProtocol(params, doc) {
			hasCodeAware = true
			break
		}
	}

	// Resolve the API key for endpoint queries (model metadata lookup).
	resolvedAPIKey := llmAPIKey
	if resolvedAPIKey == "" && llmAPIKeyEnv != "" {
		resolvedAPIKey = os.Getenv(llmAPIKeyEnv)
	}

	// Resolve context window and output tokens at runtime. These are not
	// stored in config — they are auto-detected from the /v1/models endpoint
	// or overridden via flags. Auto-detection only runs when needed (reference
	// revision or code-aware comment revision).
	//
	// Main model context window: needed for reference revision, code comment
	// revision when the code model is the same, or when the user explicitly
	// provides --context-tokens (which should apply to all prose revision).
	needsMainContextWindow := refPack != nil || (hasCodeAware && codeModel == llmModel) || params.ContextTokens > 0
	if needsMainContextWindow {
		if params.ContextTokens > 0 {
			llmCfg.ContextWindowTokens = params.ContextTokens
		} else {
			cw, lookupErr := llm.LookupContextWindow(llmBaseURL, resolvedAPIKey, llmModel, llmTimeout)
			if lookupErr != nil {
				// Auto-detection failed. For reference revision this is fatal;
				// for code-comment-only revision, fall back to legacy byte budget.
				if refPack != nil {
					return fmt.Errorf("%w: could not auto-detect context window for model %q from %s/models: %v; use --context-tokens to specify it explicitly", ErrLLMConfigRequired, llmModel, llmBaseURL, lookupErr)
				}
			} else {
				llmCfg.ContextWindowTokens = cw
			}
		}
		if llmCfg.ContextWindowTokens > 0 {
			if params.OutputTokens > 0 {
				llmCfg.MaxOutputTokens = params.OutputTokens
			} else {
				llmCfg.MaxOutputTokens = llm.DefaultMaxOutputTokens
			}
			if llmCfg.MaxOutputTokens >= llmCfg.ContextWindowTokens {
				return fmt.Errorf("%w: output tokens (%d) must be less than context window (%d)", ErrLLMConfigRequired, llmCfg.MaxOutputTokens, llmCfg.ContextWindowTokens)
			}
		}
		if refPack != nil && llmCfg.ContextWindowTokens == 0 {
			return fmt.Errorf("%w: reference revision requires a known context window for model %q, but none was provided by --context-tokens or the /v1/models endpoint; use --context-tokens to specify it explicitly", ErrLLMConfigRequired, llmModel)
		}
	}

	// Code model config: when the code model differs from the main model, build
	// a separate config and resolve its context window independently.
	codeLlmCfg := llmCfg
	if hasCodeAware && codeModel != llmModel {
		codeLlmCfg = llm.Config{
			BaseURL:      llmBaseURL,
			Model:        codeModel,
			APIKey:       llmAPIKey,
			APIKeyEnv:    llmAPIKeyEnv,
			Timeout:      llmTimeout,
			ResponseMode: llmResponseMode,
		}
		if params.ContextTokens > 0 {
			codeLlmCfg.ContextWindowTokens = params.ContextTokens
		} else {
			cw, lookupErr := llm.LookupContextWindow(llmBaseURL, resolvedAPIKey, codeModel, llmTimeout)
			if lookupErr != nil {
				// Fall back to legacy byte budget for code comment revision.
				fmt.Fprintf(os.Stderr, "Warning: could not auto-detect context window for code model %q: %v\n", codeModel, lookupErr)
			} else {
				codeLlmCfg.ContextWindowTokens = cw
			}
		}
		if codeLlmCfg.ContextWindowTokens > 0 {
			if params.OutputTokens > 0 {
				codeLlmCfg.MaxOutputTokens = params.OutputTokens
			} else {
				codeLlmCfg.MaxOutputTokens = llm.DefaultMaxOutputTokens
			}
			if codeLlmCfg.MaxOutputTokens >= codeLlmCfg.ContextWindowTokens {
				return fmt.Errorf("%w: output tokens (%d) must be less than context window (%d) for code model %q", ErrLLMConfigRequired, codeLlmCfg.MaxOutputTokens, codeLlmCfg.ContextWindowTokens, codeModel)
			}
		}
	}

	type revisionPlan struct {
		doc       *document.Document
		chunks    []document.ChunkRange
		codeAware bool
		noTargets bool
	}
	plans := make([]revisionPlan, 0, len(docs))
	totalRequests := 0
	for _, doc := range docs {
		codeAware := usesCodeCommentProtocol(params, doc)
		noTargets := false
		var chunks []document.ChunkRange
		if codeAware {
			language, _ := codecomment.DetectLanguage(doc.Source)
			catalog, catalogErr := codecomment.Extract(doc.Source, language, []byte(doc.Content))
			if catalogErr != nil {
				return fmt.Errorf("cataloging comments in %q: %w", doc.Source, catalogErr)
			}
			noTargets = len(catalog.Comments) == 0
			if !noTargets {
				// Code-aware review is intentionally one whole-source request. Its
				// request-level context budget is checked by llm.ReviseCodeComments.
				chunks = []document.ChunkRange{{StartByte: 0, EndByte: len(doc.Content)}}
			}
		} else {
			chunks, err = planBudgetedChunks(doc, refPack, llmCfg, maxRequests, res, findings, terms)
			if err != nil {
				return fmt.Errorf("planning chunks for %q: %w", doc.Source, err)
			}
		}
		totalRequests += len(chunks)
		plans = append(plans, revisionPlan{doc: doc, chunks: chunks, codeAware: codeAware, noTargets: noTargets})
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
	discardedFindings := 0
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
		if plan.noTargets {
			coverage.AnalyzedBytes = len(plan.doc.Content)
		}
		if plan.doc.Format == document.FormatHTML {
			coverage.SourceFormat = "html"
			coverage.RangeBasis = "visible_text"
		}
		for _, chunk := range plan.chunks {
			requestsMade++
			remainingPrimaryRequests--
			coverage.ModelRequests++
			var revResp *report.ReviseResponse
			var callErr error
			if plan.codeAware {
				revResp, callErr = llm.ReviseCodeComments(context.Background(), codeLlmCfg, plan.doc, res)
			} else {
				revResp, callErr = llm.ReviseChunk(context.Background(), llmCfg, plan.doc, res, docFindings, terms, chunk.StartByte, chunk.EndByte, refPack)
			}
			if callErr != nil && errors.Is(callErr, llm.ErrInvalidModelResponse) && (llmResponseMode == "json_object" || llmResponseMode == "json_schema") && requestsMade+remainingPrimaryRequests < maxRequests {
				activeCfg := llmCfg
				if plan.codeAware {
					activeCfg = codeLlmCfg
				}
				fallbackCfg := activeCfg
				fallbackCfg.ResponseMode = "prompt_json"
				requestsMade++
				coverage.ModelRequests++
				var fallbackResp *report.ReviseResponse
				var fallbackErr error
				if plan.codeAware {
					fallbackResp, fallbackErr = llm.ReviseCodeComments(context.Background(), fallbackCfg, plan.doc, res)
				} else {
					fallbackResp, fallbackErr = llm.ReviseChunk(context.Background(), fallbackCfg, plan.doc, res, docFindings, terms, chunk.StartByte, chunk.EndByte, refPack)
				}
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
			discardedFindings += revResp.DiscardedFindings
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
	var referenceContext *report.ReferenceContext
	if refPack != nil {
		referenceContext = &report.ReferenceContext{
			Paths:               params.ReferencePaths,
			Files:               make([]string, len(refPack.Entries)),
			InputBytes:          refPack.InputBytes,
			IncludedBytes:       refPack.IncludedBytes,
			Complete:            refPack.Complete,
			Warnings:            refPack.Warnings,
			ContextWindowTokens: llmCfg.ContextWindowTokens,
			MaxOutputTokens:     llmCfg.MaxOutputTokens,
		}
		for i, e := range refPack.Entries {
			referenceContext.Files[i] = e.SourcePath
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
		DiscardedFindings: discardedFindings,
		Errors:            revisionErrors,
		Revisions:         allRevisions,
		ReferenceContext:  referenceContext,
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

// usesCodeCommentProtocol selects lexer-owned comment IDs only for explicit file
// input. Stdin, --text, and unsupported extensions deliberately retain the
// legacy prose-style code-comment path.
func usesCodeCommentProtocol(params ReviseParams, doc *document.Document) bool {
	return usesCodeCommentCatalog(params.Kind, params.Stdin, params.Text, doc)
}

func usesCodeCommentCatalog(kind string, stdin bool, text *string, doc *document.Document) bool {
	if kind != guidance.KindCodeComment || stdin || text != nil || doc == nil {
		return false
	}
	_, ok := codecomment.DetectLanguage(doc.Source)
	return ok
}

// planBudgetedChunks plans chunks for a single document, taking into account
// the reference pack overhead and token budget when configured.
// Returns the chunk ranges and error.
// When cfg.ContextWindowTokens is 0 or refPack is nil, falls back to legacy
// byte-budget chunking.
func planBudgetedChunks(doc *document.Document, refPack *reference.Pack, cfg llm.Config, maxRequests int, res *profile.Resolution, findings []report.Finding, terms []config.TermEntry) ([]document.ChunkRange, error) {
	// Legacy path: no context window or no references.
	if cfg.ContextWindowTokens == 0 || refPack == nil {
		chunks := document.ChunkRanges(doc, defaultRevisionChunkBytes)
		return chunks, nil
	}

	content := doc.AnalysisContent()
	if len(content) == 0 {
		// Empty documents produce no chunks (ChunkRanges parity), so no model
		// call is made for them.
		return []document.ChunkRange{}, nil
	}

	// 2a. Compute base overhead by building the prompt for a minimal (1 byte)
	// excerpt. This validates that the system/reference/schema overhead alone
	// leaves at least minEditableSourceTokens; BuildBudgetedPrompt returns the
	// actionable configuration error otherwise.
	minChunkBytes := llm.MinEditableSourceTokens * llm.EstimatedBytesPerToken
	oneByteExcerpt := llm.NewChunkExcerpt(doc, 0, 1)
	sysPrompt, userContent, _, err := llm.BuildBudgetedPrompt(doc, res, findings, terms, oneByteExcerpt, refPack, cfg)
	if err != nil {
		return nil, fmt.Errorf("reference overhead exceeds context window: %w", err)
	}

	// Serialize the base request (with the response format/schema included) so
	// the initial chunk size derives from the same request shape that
	// BuildBudgetedPrompt measures.
	baseSerializedBytes, marshalErr := llm.SerializeRequestBytes(cfg, sysPrompt, userContent)
	if marshalErr != nil {
		return nil, fmt.Errorf("budget calculation: %w", marshalErr)
	}
	basePromptTokens := int(math.Ceil(float64(baseSerializedBytes) / float64(llm.EstimatedBytesPerToken)))

	maxOutputTokens := cfg.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = llm.DefaultMaxOutputTokens
	}

	availableSourceBudget := cfg.ContextWindowTokens - basePromptTokens - maxOutputTokens - llm.BudgetSafetyTokens
	if availableSourceBudget < llm.MinEditableSourceTokens {
		return nil, fmt.Errorf(
			"revision context requires %d estimated input tokens for system/reference material and output reservation, "+
				"leaving %d estimated tokens for editable source; "+
				"use a larger --context-tokens value or use a smaller reference set",
			basePromptTokens+maxOutputTokens, availableSourceBudget)
	}

	// 2b. Convert available budget to bytes.
	availableSourceBytes := availableSourceBudget * llm.EstimatedBytesPerToken

	// 2c. Use the smaller of availableSourceBytes and defaultRevisionChunkBytes.
	chunkSize := availableSourceBytes
	if chunkSize > defaultRevisionChunkBytes {
		chunkSize = defaultRevisionChunkBytes
	}
	if chunkSize < minChunkBytes {
		chunkSize = minChunkBytes
	}

	var chunks []document.ChunkRange
	currentPos := 0

	// Iteratively build chunks: each chunk starts where the previous ended.
	// Every candidate is validated with the complete prompt for that exact
	// chunk (lint findings, dictionary guidance, glossary matches, and HTML
	// projection can change overhead). When a candidate does not fit, its end
	// is moved backward to the previous UTF-8-safe block boundary and the
	// candidate is rebuilt until it fits. A final fragment smaller than
	// minEditableSourceTokens is still taken whole (never silently dropped).
	for currentPos < len(content) {
		chunkStart := currentPos
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(content) {
			chunkEnd = len(content)
		}

		fitted := false
		var lastBudgetErr error
		for {
			chunkExcerpt := llm.NewChunkExcerpt(doc, chunkStart, chunkEnd)
			_, _, _, budgetErr := llm.BuildBudgetedPrompt(doc, res, findings, terms, chunkExcerpt, refPack, cfg)
			if budgetErr == nil {
				fitted = true
				break
			}
			lastBudgetErr = budgetErr
			// A fragment below the minimum editable-source size cannot shrink
			// (that would drop coverage); validate it once and surface its error.
			if chunkEnd < chunkStart+minChunkBytes {
				break
			}

			// Shrink: move the end back to the previous double-newline or
			// newline boundary (a UTF-8-safe position, since '\n' is ASCII).
			window := content[chunkStart:chunkEnd]
			newEnd := chunkEnd
			if idx := strings.LastIndex(window, "\n\n"); idx >= 0 {
				newEnd = chunkStart + idx + 2
			} else if idx := strings.LastIndexByte(window, '\n'); idx >= 0 {
				newEnd = chunkStart + idx + 1
			} else {
				// No block boundary found; fall back to a UTF-8-safe midpoint
				// rather than halving into the middle of a rune.
				mid := chunkStart + (chunkEnd-chunkStart)/2
				for mid > chunkStart && !utf8.RuneStart(content[mid]) {
					mid--
				}
				if mid <= chunkStart {
					mid = chunkStart + 1
				}
				newEnd = mid
			}

			if newEnd < chunkStart+minChunkBytes || newEnd >= chunkEnd {
				break
			}
			chunkEnd = newEnd
		}

		if !fitted {
			if chunkEnd < chunkStart+minChunkBytes {
				// The final fragment cannot fit even though it is below the minimum
				// editable-source size; fail before any model call.
				return nil, fmt.Errorf(
					"cannot fit final fragment of %d bytes for document %q within the available context window budget: %v. "+
						"Consider increasing --context-tokens or reducing reference content.",
					chunkEnd-chunkStart, doc.Source, lastBudgetErr)
			}
			return nil, fmt.Errorf(
				"cannot fit any chunk of at least %d bytes for document %q "+
					"within the available context window budget. "+
					"Consider increasing --context-tokens or reducing reference content.",
				minChunkBytes, doc.Source)
		}

		chunks = append(chunks, document.ChunkRange{StartByte: chunkStart, EndByte: chunkEnd})
		currentPos = chunkEnd
	}

	return chunks, nil
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
