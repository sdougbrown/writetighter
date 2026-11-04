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

// commands declares the migrated surface. Handle funcs are resolved at
// dispatch time through these declarations.
var commands = []*clicontract.Command{
	{
		Name:  "version",
		Short: "Print version and embedded profile information",
		Flags: []clicontract.Flag{
			{Name: "json", Kind: clicontract.Bool, Usage: "output version information as JSON (default: human-readable)"},
		},
		Handle: handleVersion,
	},
}

// CommandShorts reports the declared short description of every migrated
// top-level command. Help surfaces that still render from the
// hand-rolled texts claim to describe the same commands; cmd's tests
// compare this map against those texts so the two cannot drift during
// the migration.
func CommandShorts() map[string]string {
	m := make(map[string]string, len(commands))
	for _, c := range commands {
		m[c.Name] = c.Short
	}
	return m
}

// RootShort is the root tagline. The hand-rolled root help renders the
// same line (with the "writetighter — " prefix) until every command
// migrates; cmd's drift test pins the two so they cannot diverge.
const RootShort = "deterministic and contextual prose revision for technical documentation"

func appFor() *clicontract.App {
	return &clicontract.App{
		Name:      "writetighter",
		Version:   app.Version,
		RootShort: RootShort,
		// The root is a stub until every command migrates; the hand-rolled
		// dispatch owns root help and unknown-command handling meanwhile.
		BareHelp: true,
		Commands: commands,
	}
}

// loadEmbedded provides a package-level hook so tests can replace the
// embedded profile loader to simulate a no-profile state.
var loadEmbedded = profile.LoadEmbedded

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

type cliAdapter struct{}

// Run executes argv (program name excluded) through the declared surface.
// Commands that have not migrated are not declared here; callers route
// only migrated commands.
func Run(argv []string, stdout, stderr io.Writer) int {
	return clicontract.Run(appFor(), cliAdapter{}, argv, stdout, stderr)
}
