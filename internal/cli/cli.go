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
	_, textSet := inv.Supplied["text"]
	if textSet {
		text := inv.Flags["text"]
		params.Text = &text
	}
	switch {
	case (params.Stdin && (len(params.Paths) > 0 || textSet)) || (textSet && len(params.Paths) > 0):
		return nil, &clicontract.UsageError{Message: "paths, --stdin, and --text are mutually exclusive"}
	case !params.Stdin && !textSet && len(params.Paths) == 0:
		return nil, &clicontract.UsageError{Message: "no input specified"}
	}
	if err := app.New().RunLintTo(stdout, params); err != nil {
		return nil, err
	}
	return lintResult{}, nil
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
