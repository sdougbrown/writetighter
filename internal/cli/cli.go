// Package cli implements the writetighter command boundary on the
// clicontract library: the declared command surface, the output
// vocabulary, and the exit-code mapping. Commands migrate here one at a
// time; cmd/writetighter/main.go keeps the hand-rolled dispatch for the
// commands that have not migrated yet.
//
// Migrated commands print the library's dispatch-level diagnostics
// (for example, "version: unknown flag: --wat") with no stdout on
// failure, not the hand-rolled commands'
// "Run `writetighter <cmd> --help` for usage." hint. The hint is retired
// command by command as the migration completes.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	clicontract "github.com/sdougbrown/cli-contract"

	"github.com/sdougbrown/writetighter/internal/app"
	"github.com/sdougbrown/writetighter/internal/guidance"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/setup"
)

// Exit codes are owned by internal/cli. The values match the contract
// documented in cmd/writetighter/main.go (0 success, 1 lint fail
// threshold, 2 usage/configuration/input failure, 3 model failure);
// every command references these constants as it migrates, so the
// vocabulary has one owner. Signal handling is the library's:
// migrated commands exit 130/143 on SIGINT/SIGTERM.
const (
	ExitOK            = 0
	ExitFailThreshold = 1
	ExitUsage         = 2
	ExitModel         = 3
)

// buildCommands declares the migrated surface, closing over the stdout
// writer where a command emits through it; Handle funcs are resolved at
// dispatch time through these declarations.
func buildCommands(stdout io.Writer) []*clicontract.Command {
	return []*clicontract.Command{
		{
			Name:  "version",
			Short: "Print version and embedded profile information",
			Flags: []clicontract.Flag{
				{Name: "json", Kind: clicontract.Bool, Usage: "output version information as JSON (default: human-readable)"},
			},
			Handle: handleVersion,
		},
		{
			Name:        "lint",
			Short:       "Run deterministic profile rules (no model access)",
			Positionals: []clicontract.Positional{{Name: "paths", Variadic: true}},
			Long:        "Checks input against the active profile's static rules. No model access is needed.\n\nINPUT\n  Provide input via file paths, --stdin, or --text (mutually exclusive).\n\nEXAMPLES\n  writetighter lint README.md\n  writetighter lint --stdin < README.md\n  writetighter lint --text \"Short text.\" --kind procedure\n  writetighter lint --format json docs/*.md --fail-on warning\n",
			Flags: []clicontract.Flag{
				{Name: "stdin", Kind: clicontract.Bool, Usage: "Read input from stdin"},
				{Name: "text", Kind: clicontract.String, Usage: "Lint the provided text directly"},
				{Name: "kind", Kind: clicontract.String, Usage: "Document kind (choices: description, procedure, pr, code-comment, reference, decision, incident, agent-instruction, status-update)", Default: "description"},
				{Name: "profile", Kind: clicontract.String, Usage: "Profile id@version (default: discovered or user config)"},
				{Name: "config", Kind: clicontract.String, Usage: "Path to .writetighter.toml (default: auto-discovered)"},
				{Name: "format", Kind: clicontract.String, Usage: "Output format: human, json, agent", Default: "human"},
				{Name: "fail-on", Kind: clicontract.String, Usage: "Fail when findings reach this severity (choices: none, warning, error)", Default: "none"},
				{Name: "git-compare", Kind: clicontract.String, Usage: "Git revision to compare against for unfamiliar-term detection (only valid with file paths; advisory, candidate/info)"},
			},
			Handle: func(ctx context.Context, inv clicontract.Invocation, stderr io.Writer) (any, error) {
				return handleLint(stdout, ctx, inv, stderr)
			},
		},
		{
			Name:        "revise",
			Short:       "Run contextual model-based revision (requires model config)",
			Positionals: []clicontract.Positional{{Name: "paths", Variadic: true}},
			Long:        "Sends input to an OpenAI-compatible model with revision guidance and returns rewritten prose. Requires [llm] configuration in the user config.\n\nINPUT\n  Provide input via file paths, --stdin, or --text (mutually exclusive).\n\nCONFIGURATION\n  Revise requires [llm] model settings. Run \"writetighter config\" to configure.\n  If config is missing and stdin is a terminal, interactive setup is offered\n  automatically; otherwise the command exits with an error.\n\n  Reference revision auto-detects the context window from the\n  /v1/models endpoint. Use --context-tokens to override it.\n\nEXAMPLES\n  writetighter revise --text \"Short text.\" --kind procedure\n  writetighter revise --stdin < README.md\n  writetighter revise docs/*.md --format human\n  writetighter revise docs/*.md --reference style-guide.md\n",
			Flags: []clicontract.Flag{
				{Name: "stdin", Kind: clicontract.Bool, Usage: "Read input from stdin"},
				{Name: "text", Kind: clicontract.String, Usage: "Revise the provided text directly"},
				{Name: "kind", Kind: clicontract.String, Usage: "Document kind (choices: description, procedure, pr, code-comment, reference, decision, incident, agent-instruction, status-update)", Default: "description"},
				{Name: "profile", Kind: clicontract.String, Usage: "Profile id@version (default: discovered or user config)"},
				{Name: "config", Kind: clicontract.String, Usage: "Path to .writetighter.toml (default: auto-discovered)"},
				{Name: "format", Kind: clicontract.String, Usage: "Output format: json, human", Default: "json"},
				{Name: "reference", Kind: clicontract.Repeat, Usage: "Path to a reference file or directory (repeatable); reference content provides broader context for revision decisions"},
				{Name: "model", Kind: clicontract.String, Usage: "Override the configured model for this run"},
				{Name: "code-model", Kind: clicontract.String, Usage: "Override the model used for code-aware comment revision"},
				{Name: "context-tokens", Kind: clicontract.Int, Usage: "Override the model context window (tokens); 0 = auto-detect from /v1/models"},
				{Name: "output-tokens", Kind: clicontract.Int, Usage: "Override max output tokens sent to the API; 0 = default when context window is known"},
			},
			Handle: func(ctx context.Context, inv clicontract.Invocation, stderr io.Writer) (any, error) {
				return handleRevise(stdout, stderr, ctx, inv)
			},
		},
		{
			Name:        "rewrite",
			Short:       "Run whole-passage contextual rewrite (requires model config)",
			Positionals: []clicontract.Positional{{Name: "paths", Variadic: true}},
			Long:        "Sends the full input as one model request and returns the complete rewritten passage, not surgical findings. Deterministic lint runs first and its findings are passed to the model as context. If the model call fails, the response is empty, or protected-content validation fails, the original text is returned.\n\nUnlike \"revise\", which returns individual suggestions for interactive review, \"rewrite\" produces one coherent tightened passage for non-interactive contexts (hooks, CI, piping into other tools). It never asks clarification questions; ambiguous sections are preserved as-is rather than guessed.\n\nINPUT\n  Provide input via file paths, --stdin, or --text (mutually exclusive).\n\nCONFIGURATION\n  Rewrite requires [llm] model settings. Run \"writetighter config\" to configure.\n  If config is missing and stdin is a terminal, interactive setup is offered\n  automatically; otherwise the command exits with an error.\n\nEXAMPLES\n  writetighter rewrite --text \"Short text.\" --kind procedure\n  writetighter rewrite --stdin < README.md\n  printf '%s' \"$MESSAGE\" | writetighter rewrite --stdin --kind description\n  writetighter rewrite docs/intro.md --format json\n",
			Flags: []clicontract.Flag{
				{Name: "stdin", Kind: clicontract.Bool, Usage: "Read input from stdin"},
				{Name: "text", Kind: clicontract.String, Usage: "Rewrite the provided text directly"},
				{Name: "kind", Kind: clicontract.String, Usage: "Document kind (choices: description, procedure, pr, code-comment, reference, decision, incident, agent-instruction, status-update)", Default: "description"},
				{Name: "profile", Kind: clicontract.String, Usage: "Profile id@version (default: discovered or user config)"},
				{Name: "config", Kind: clicontract.String, Usage: "Path to .writetighter.toml (default: auto-discovered)"},
				{Name: "format", Kind: clicontract.String, Usage: "Output format: text, json, human", Default: "text"},
				{Name: "model", Kind: clicontract.String, Usage: "Override the configured model for this run"},
			},
			Handle: func(ctx context.Context, inv clicontract.Invocation, stderr io.Writer) (any, error) {
				return handleRewrite(stdout, stderr, ctx, inv)
			},
		},
		{
			Name:        "explain",
			Short:       "Explain a lint rule",
			Positionals: []clicontract.Positional{{Name: "rule-id", Required: true}},
			Long:        "Prints information about a named lint rule from the active profile.\n\nEXAMPLES\n  writetighter explain CORE.SENTENCE_LENGTH\n  writetighter explain CORE.SENTENCE_LENGTH --format json\n",
			Flags: []clicontract.Flag{
				{Name: "profile", Kind: clicontract.String, Usage: "Profile id@version (default: discovered or user config)"},
				{Name: "format", Kind: clicontract.String, Usage: "Output format: human, json", Default: "human"},
			},
			Handle: func(ctx context.Context, inv clicontract.Invocation, stderr io.Writer) (any, error) {
				return handleExplain(stdout, inv)
			},
		},
		{
			Name:  "prompt",
			Short: "Export core and kind-specific revision guidance",
			Flags: []clicontract.Flag{
				{Name: "kind", Kind: clicontract.String, Usage: "Document kind (choices: description, procedure, pr, code-comment, reference, decision, incident, agent-instruction, status-update)", Default: "description"},
				{Name: "format", Kind: clicontract.String, Usage: "Output format: human, json", Default: "human"},
			},
			Handle: handlePrompt,
		},
	}
}

// CommandShorts reports the declared short description of every migrated
// top-level command. Help surfaces that still render from the
// hand-rolled texts claim to describe the same commands; cmd's tests
// compare this map against those texts so the two cannot drift during
// the migration.
func CommandShorts() map[string]string {
	cs := buildCommands(io.Discard)
	m := make(map[string]string, len(cs))
	for _, c := range cs {
		m[c.Name] = c.Short
	}
	return m
}

// RootShort is the root tagline. The hand-rolled root help renders the
// same line (with the "writetighter — " prefix) until every command
// migrates; cmd's drift test pins the two so they cannot diverge.
const RootShort = "deterministic and contextual prose revision for technical documentation"

func appFor(stdout io.Writer) *clicontract.App {
	return &clicontract.App{
		Name:      "writetighter",
		Version:   app.Version,
		RootShort: RootShort,
		// The root is a stub until every command migrates; the hand-rolled
		// dispatch owns root help and unknown-command handling meanwhile.
		BareHelp: true,
		Commands: buildCommands(stdout),
	}
}

// loadEmbedded provides a package-level hook so tests can replace the
// embedded profile loader to simulate a no-profile state.
var loadEmbedded = profile.LoadEmbedded

// runWithMissingConfig maps a RunXxxTo error: a missing model
// configuration offers the interactive wizard on a terminal and retries
// once via retry; any other error passes through, and a non-terminal
// missing config maps to a usage error (exit 2).
func runWithMissingConfig(ctx context.Context, stderr io.Writer, stdinFlag bool, err error, retry func() error) error {
	if err == nil {
		return nil
	}
	if !errors.Is(err, app.ErrLLMConfigRequired) {
		return err
	}
	if stdinFlag || !setup.StdinIsTerminal() {
		// Hint line retired with the migration; see package doc.
		return &clicontract.UsageError{Message: err.Error()}
	}
	fmt.Fprintf(stderr, "%v\nStarting interactive configuration.\n", err)
	if wizardErr := setup.RunInteractive(ctx); wizardErr != nil {
		return &clicontract.UsageError{Message: "config failed: " + wizardErr.Error()}
	}
	return retry()
}

// resolveInputMode validates the input-mode contract shared by every
// migrated document command (lint, revise, rewrite): paths, --stdin and
// --text are mutually exclusive, and at least one input is required.
// Returns the --text value when it was given.
func resolveInputMode(inv clicontract.Invocation) (*string, error) {
	_, textSet := inv.Supplied["text"]
	var text *string
	if textSet {
		t := inv.Flags["text"]
		text = &t
	}
	switch {
	case (inv.Bools["stdin"] && (len(inv.Args) > 0 || textSet)) || (textSet && len(inv.Args) > 0):
		return nil, &clicontract.UsageError{Message: "paths, --stdin, and --text are mutually exclusive"}
	case !inv.Bools["stdin"] && !textSet && len(inv.Args) == 0:
		return nil, &clicontract.UsageError{Message: "no input specified"}
	}
	return text, nil
}

type lintResult struct{}

func handleLint(stdout io.Writer, _ context.Context, inv clicontract.Invocation, _ io.Writer) (any, error) {
	params := app.LintParams{
		Paths:      inv.Args,
		Stdin:      inv.Bools["stdin"],
		Kind:       inv.Flags["kind"],
		Profile:    inv.Flags["profile"],
		ConfigPath: inv.Flags["config"],
		Format:     inv.Flags["format"],
		FailOn:     inv.Flags["fail-on"],
		GitCompare: inv.Flags["git-compare"],
	}
	textArg, err := resolveInputMode(inv)
	if err != nil {
		return nil, err
	}
	params.Text = textArg
	if err := app.New().RunLintTo(stdout, params); err != nil {
		return nil, err
	}
	return lintResult{}, nil
}

type explainResult struct{}

func handleExplain(stdout io.Writer, inv clicontract.Invocation) (any, error) {
	if err := app.New().RunExplainTo(stdout, inv.Args[0], inv.Flags["profile"], inv.Flags["format"]); err != nil {
		return nil, err
	}
	return explainResult{}, nil
}

type rewriteResult struct{}

func handleRewrite(stdout, stderr io.Writer, ctx context.Context, inv clicontract.Invocation) (any, error) {
	params := app.RewriteParams{
		Paths:      inv.Args,
		Stdin:      inv.Bools["stdin"],
		Kind:       inv.Flags["kind"],
		Profile:    inv.Flags["profile"],
		ConfigPath: inv.Flags["config"],
		Format:     inv.Flags["format"],
		Model:      inv.Flags["model"],
	}
	textArg, err := resolveInputMode(inv)
	if err != nil {
		return nil, err
	}
	params.Text = textArg
	err = app.New().RunRewriteTo(stdout, params)
	err = runWithMissingConfig(ctx, stderr, params.Stdin, err, func() error { return app.New().RunRewriteTo(stdout, params) })
	if err != nil {
		// ErrRewriteFailed maps to exit 3 via the adapter's sentinel mapping;
		// every other failure maps to exit 2.
		return nil, err
	}
	return rewriteResult{}, nil
}

type reviseResult struct{}

func handleRevise(stdout, stderr io.Writer, ctx context.Context, inv clicontract.Invocation) (any, error) {
	params := app.ReviseParams{
		Paths:          inv.Args,
		Stdin:          inv.Bools["stdin"],
		Kind:           inv.Flags["kind"],
		Profile:        inv.Flags["profile"],
		ConfigPath:     inv.Flags["config"],
		Format:         inv.Flags["format"],
		ReferencePaths: inv.Events,
		Model:          inv.Flags["model"],
		CodeModel:      inv.Flags["code-model"],
		ContextTokens:  inv.Ints["context-tokens"],
		OutputTokens:   inv.Ints["output-tokens"],
	}
	textArg, err := resolveInputMode(inv)
	if err != nil {
		return nil, err
	}
	params.Text = textArg
	err = app.New().RunReviseTo(stdout, params)
	err = runWithMissingConfig(ctx, stderr, params.Stdin, err, func() error { return app.New().RunReviseTo(stdout, params) })
	if err != nil {
		// ErrReviseFailed maps to exit 3 via the adapter's sentinel mapping;
		// every other failure maps to exit 2.
		return nil, err
	}
	return reviseResult{}, nil
}

type promptResult struct {
	format string
	rubric *guidance.Set
}

func handlePrompt(_ context.Context, inv clicontract.Invocation, _ io.Writer) (any, error) {
	rubric, format, err := app.PromptData(app.PromptParams{Kind: inv.Flags["kind"], Format: inv.Flags["format"]})
	if err != nil {
		return nil, err
	}
	return promptResult{format: format, rubric: rubric}, nil
}

func handleVersion(_ context.Context, inv clicontract.Invocation, _ io.Writer) (any, error) {
	r, _ := loadEmbedded()
	profiles := []map[string]any{}
	if r != nil {
		profiles = append(profiles, map[string]any{"id": string(r.ID), "version": string(r.Version), "sha256": r.SHA256})
	}
	format := "human"
	if inv.Bools["json"] {
		format = "json"
	}
	return versionResult{format: format, version: app.Version, commit: app.Commit, profiles: profiles}, nil
}

type versionResult struct {
	format   string
	version  string
	commit   string
	profiles []map[string]any
}

// failureResult carries a dispatch-level failure. Run prints the
// diagnostic to stderr itself; Write deliberately emits nothing on
// stdout, matching the hand-rolled dispatch's stdout contract.
type failureResult struct{}

func (cliAdapter) ErrorResult(_ string, _ error) any { return failureResult{} }

func (cliAdapter) Write(w io.Writer, result any) error {
	switch r := result.(type) {
	case explainResult:
		// The rule description was written via the dispatch stdout writer;
		// nothing else to emit.
		return nil
	case rewriteResult:
		// The rendered rewrite document was written via the dispatch
		// stdout writer; nothing else to emit.
		return nil
	case reviseResult:
		// The rendered revision document was written via the dispatch
		// stdout writer; nothing else to emit.
		return nil
	case lintResult:
		// Findings were already written to stdout by RunLint; nothing
		// else to emit.
		return nil
	case promptResult:
		if r.format == "json" {
			return json.NewEncoder(w).Encode(r.rubric)
		}
		_, err := io.WriteString(w, app.PromptText(r.rubric))
		return err
	case versionResult:
		if r.format == "json" {
			payload := map[string]any{"version": r.version, "commit": r.commit, "embedded_profiles": r.profiles}
			return json.NewEncoder(w).Encode(payload)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "writetighter %s", r.version)
		if r.commit != "" && r.commit != "unknown" {
			fmt.Fprintf(&b, " (commit %s)", r.commit)
		}
		b.WriteString("\n")
		for _, p := range r.profiles {
			fmt.Fprintf(&b, "embedded profile: %s@%s\n", p["id"], p["version"])
		}
		_, err := io.WriteString(w, b.String())
		return err
	case failureResult, nil:
		// Dispatch-level failure (or a handler that failed without producing
		// a result): the diagnostic is on stderr; stdout stays empty.
		return nil
	default:
		return fmt.Errorf("cli: unhandled result type %T", result)
	}
}

func (cliAdapter) ExitCode(_ clicontract.Invocation, _ any, handlerErr error) int {
	if handlerErr == nil {
		return ExitOK
	}
	// Sentinel mapping, not a per-command switch: lint's fail threshold
	// and the model failures identify themselves, so migrated commands
	// inherit the codes without a mapping arm per command.
	switch {
	case errors.Is(handlerErr, app.ErrFailThreshold):
		return ExitFailThreshold
	case errors.Is(handlerErr, app.ErrReviseFailed),
		errors.Is(handlerErr, app.ErrRewriteFailed):
		return ExitModel
	default:
		// version has no model path; every other failure here is an
		// input or configuration problem.
		return ExitUsage
	}
}

func (cliAdapter) Deadline(ctx context.Context, _ clicontract.Invocation) (context.Context, context.CancelFunc) {
	return ctx, func() {}
}

func (cliAdapter) BeforeHandler(_ clicontract.Invocation, _ io.Writer) {}

func (cliAdapter) Validate(_ clicontract.Invocation) error { return nil }

// HandlerDiagnostic suppresses the diagnostic line for the fail-threshold
// sentinel: reaching --fail-on is the command's documented success-adjacent
// signal and the hand-rolled dispatch printed nothing on exit 1. Other
// handler failures print the error message, matching the old behavior.
func (cliAdapter) HandlerDiagnostic(_ clicontract.Invocation, err error) string {
	if errors.Is(err, app.ErrFailThreshold) {
		return ""
	}
	return err.Error()
}

type cliAdapter struct{}

// Run executes argv (program name excluded) through the declared surface.
// Commands that have not migrated are not declared here; callers route
// only migrated commands.
func Run(argv []string, stdout, stderr io.Writer) int {
	return clicontract.Run(appFor(stdout), cliAdapter{}, argv, stdout, stderr)
}
