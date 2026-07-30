package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sdougbrown/writetighter/internal/app"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/setup"
	"golang.org/x/term"
)

func main() { os.Exit(run(os.Args[1:])) }

type optionalStringFlag struct {
	value string
	set   bool
}

func (f *optionalStringFlag) String() string { return f.value }
func (f *optionalStringFlag) Set(value string) error {
	f.value = value
	f.set = true
	return nil
}

// Exit codes:
// 0: command completed successfully.
// 1: lint findings reached --fail-on.
// 2: usage, configuration, profile, or input failure.
// 3: a required model call or model response failed.
func run(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printHelp("")
		return 0
	}
	switch args[0] {
	case "help":
		return runHelp(args[1:])
	case "version":
		return runVersion(args[1:])
	case "lint":
		return runLint(args[1:])
	case "revise":
		return runRevise(args[1:])
	case "prompt":
		return runPrompt(args[1:])
	case "config":
		return runConfig(args[1:])
	case "explain":
		return runExplain(args[1:])
	case "profile":
		return runProfile(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printHelp("")
		return 2
	}
}

func runHelp(args []string) int {
	if len(args) == 0 {
		printHelp("")
		return 0
	}
	printHelp(args[0])
	return 0
}

// wantsHelp reports whether the argument list contains --help or -h.
// Checked before flag parsing so it works regardless of flag/positional ordering.
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// runLint runs deterministic profile rules without model access.
func runLint(args []string) int {
	if wantsHelp(args) {
		printHelp("lint")
		return 0
	}
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	stdin := fs.Bool("stdin", false, "")
	var text optionalStringFlag
	fs.Var(&text, "text", "")
	kind := fs.String("kind", "description", "")
	profile := fs.String("profile", "", "")
	configPath := fs.String("config", "", "")
	format := fs.String("format", "human", "")
	failOn := fs.String("fail-on", "none", "")
	fs.Usage = func() { printHelp("lint") }
	if err := fs.Parse(normalizeInterspersedFlags(args)); err != nil {
		return 2
	}
	params := app.LintParams{Paths: fs.Args(), Stdin: *stdin, Kind: *kind, Profile: *profile, ConfigPath: *configPath, Format: *format, FailOn: *failOn}
	if text.set {
		params.Text = &text.value
	}
	if (params.Stdin && (len(params.Paths) > 0 || params.Text != nil)) || (params.Text != nil && len(params.Paths) > 0) {
		fmt.Fprintln(os.Stderr, "paths, --stdin, and --text are mutually exclusive")
		fmt.Fprintln(os.Stderr, "  Run `writetighter lint --help` for usage.")
		return 2
	}
	if !params.Stdin && params.Text == nil && len(params.Paths) == 0 {
		fmt.Fprintln(os.Stderr, "no input specified")
		fmt.Fprintln(os.Stderr, "  Run `writetighter lint --help` for usage.")
		return 2
	}
	err := app.New().RunLint(params)
	switch {
	case errors.Is(err, app.ErrFailThreshold):
		return 1
	case err != nil:
		usageErr(err.Error())
		return 2
	}
	return 0
}

// runRevise is the opt-in contextual revision command.
func runRevise(args []string) int {
	if wantsHelp(args) {
		printHelp("revise")
		return 0
	}
	fs := flag.NewFlagSet("revise", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	stdin := fs.Bool("stdin", false, "")
	var text optionalStringFlag
	fs.Var(&text, "text", "")
	kind := fs.String("kind", "description", "")
	profile := fs.String("profile", "", "")
	configPath := fs.String("config", "", "")
	format := fs.String("format", "json", "")
	fs.Usage = func() { printHelp("revise") }
	if err := fs.Parse(normalizeInterspersedFlags(args)); err != nil {
		return 2
	}
	params := app.ReviseParams{Paths: fs.Args(), Stdin: *stdin, Kind: *kind, Profile: *profile, ConfigPath: *configPath, Format: *format}
	if text.set {
		params.Text = &text.value
	}
	if (params.Stdin && (len(params.Paths) > 0 || params.Text != nil)) || (params.Text != nil && len(params.Paths) > 0) {
		fmt.Fprintln(os.Stderr, "paths, --stdin, and --text are mutually exclusive")
		fmt.Fprintln(os.Stderr, "  Run `writetighter revise --help` for usage.")
		return 2
	}
	if !params.Stdin && params.Text == nil && len(params.Paths) == 0 {
		fmt.Fprintln(os.Stderr, "no input specified")
		fmt.Fprintln(os.Stderr, "  Run `writetighter revise --help` for usage.")
		return 2
	}
	err := app.New().RunRevise(params)
	if errors.Is(err, app.ErrLLMConfigRequired) {
		if !params.Stdin && stdinIsTerminal() {
			fmt.Fprintf(os.Stderr, "%v\nStarting interactive configuration.\n", err)
			if code := runConfig(nil); code != 0 {
				return code
			}
			err = app.New().RunRevise(params)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
			fmt.Fprintln(os.Stderr, "  Run `writetighter config` to configure, then retry.")
			return 2
		}
	}
	if err != nil {
		// Runtime model/response failures => exit 3; configuration/input errors => exit 2.
		if errors.Is(err, app.ErrReviseFailed) {
			usageErr(err.Error())
			return 3
		}
		usageErr(err.Error())
		return 2
	}
	return 0
}

func runPrompt(args []string) int {
	if wantsHelp(args) {
		printHelp("prompt")
		return 0
	}
	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	kind := fs.String("kind", "description", "")
	format := fs.String("format", "human", "")
	fs.Usage = func() { printHelp("prompt") }
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "invalid prompt arguments")
		fmt.Fprintln(os.Stderr, "  Run `writetighter prompt --help` for usage.")
		return 2
	}
	if err := app.New().RunPrompt(app.PromptParams{Kind: *kind, Format: *format}); err != nil {
		usageErr(err.Error())
		return 2
	}
	return 0
}

func runConfig(args []string) int {
	if wantsHelp(args) {
		printHelp("config")
		return 0
	}
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { printHelp("config") }
	wizard := fs.Bool("wizard", false, "Run the interactive configuration wizard")
	fs.BoolVar(wizard, "w", false, "Run the interactive configuration wizard (short for --wizard)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "config does not accept positional arguments")
		fmt.Fprintln(os.Stderr, "  Run `writetighter config --help` for usage.")
		return 2
	}

	// If not explicitly requesting the wizard, check whether a model is configured.
	// When a model is set, show a sanitized view of the config and hint at --wizard.
	if !*wizard {
		existing, err := config.LoadUserConfig()
		if err == nil && existing != nil && existing.LLM.Model != "" {
			tomlStr, err := existing.SanitizedTOML()
			if err != nil {
				usageErr("failed to render config: " + err.Error())
				return 2
			}
			fmt.Print(tomlStr)
			if existing.LLM.APIKey != "" {
				fmt.Fprintln(os.Stderr, "\n(API key is stored in config.toml but redacted here.)")
			}
			fmt.Fprintln(os.Stderr, "To reconfigure, run: writetighter config --wizard")
			return 0
		}
	}

	// Run the interactive wizard (either --wizard was passed, or not yet configured).
	_, err := setup.Run(context.Background(), setup.Options{
		In:  os.Stdin,
		Out: os.Stderr,
		ReadSecret: func(prompt string) (string, error) {
			if !stdinIsTerminal() {
				return "", errors.New("secure API key input requires an interactive terminal")
			}
			fmt.Fprint(os.Stderr, prompt)
			secret, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			return string(secret), err
		},
	})
	if err != nil {
		usageErr("config failed: " + err.Error())
		return 2
	}
	return 0
}

func stdinIsTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// The documented CLI permits paths before flags. flag.FlagSet stops at the first
// positional argument, so normalize the small lint/revise grammar before parsing.
func normalizeInterspersedFlags(args []string) []string {
	withValue := map[string]bool{"--kind": true, "--profile": true, "--config": true, "--format": true, "--fail-on": true, "--text": true}
	var flags, paths []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			paths = append(paths, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			flags = append(flags, arg)
			if withValue[arg] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		paths = append(paths, arg)
	}
	return append(flags, paths...)
}

func runExplain(args []string) int {
	if wantsHelp(args) {
		printHelp("explain")
		return 0
	}
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { printHelp("explain") }
	profile := fs.String("profile", "", "")
	format := fs.String("format", "human", "")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 1 || (*format != "human" && *format != "json") {
		fmt.Fprintln(os.Stderr, "invalid explain arguments")
		fmt.Fprintln(os.Stderr, "  Run `writetighter explain --help` for usage.")
		return 2
	}
	if err := app.New().RunExplainWithOptions(fs.Args()[0], *profile, *format); err != nil {
		usageErr(err.Error())
		return 2
	}
	return 0
}

func runProfile(args []string) int {
	if wantsHelp(args) {
		printHelp("profile")
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "profile subcommand required")
		fmt.Fprintln(os.Stderr, "  Run `writetighter profile --help` for usage.")
		return 2
	}
	switch args[0] {
	case "install":
		if len(args) != 2 || args[1] == "" {
			usageErr("profile install requires one bundle path")
			return 2
		}
		if err := app.New().RunProfileInstall(args[1]); err != nil {
			usageErr(err.Error())
			return 2
		}
		return 0
	case "list":
		format, ok := parseProfileFormat(args[1:])
		if !ok {
			usageErr("invalid profile list arguments")
			return 2
		}
		if err := app.New().RunProfileList(format); err != nil {
			usageErr(err.Error())
			return 2
		}
		return 0
	case "verify":
		if len(args) < 2 || (len(args) >= 2 && args[1] == "") {
			usageErr("profile verify requires a profile or bundle path")
			return 2
		}
		spec := args[1]
		format, ok := parseProfileFormat(args[2:])
		if !ok {
			usageErr("invalid profile verify arguments")
			return 2
		}
		if err := app.New().RunProfileVerify(spec, format); err != nil {
			usageErr(err.Error())
			return 2
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown profile subcommand %q\n", args[0])
		printHelp("profile")
		return 2
	}
}

func runVersion(args []string) int {
	if wantsHelp(args) {
		printHelp("version")
		return 0
	}
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "")
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { printHelp("version") }
	if err := fs.Parse(args); err != nil || !*jsonFlag || len(fs.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "version requires --json")
		fmt.Fprintln(os.Stderr, "  Run `writetighter version --help` for usage.")
		return 2
	}
	r, _ := profile.LoadEmbedded()
	profiles := []any{}
	if r != nil {
		profiles = append(profiles, map[string]any{"id": string(r.ID), "version": string(r.Version), "sha256": r.SHA256})
	}
	payload := map[string]any{"version": app.Version, "commit": app.Commit, "embedded_profiles": profiles}
	_ = json.NewEncoder(os.Stdout).Encode(payload)
	return 0
}

func parseProfileFormat(args []string) (string, bool) {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {}
	format := fs.String("format", "human", "")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 0 {
		return "", false
	}
	return *format, *format == "human" || *format == "json"
}

func usageErr(msg string) { fmt.Fprintln(os.Stderr, msg) }

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

func printHelp(command string) {
	text, ok := helpTexts[command]
	if !ok {
		fmt.Fprintf(os.Stdout, "no help available for %q\n\n", command)
		text = helpTexts[""]
	}
	fmt.Fprint(os.Stdout, text)
}

const mainHelp = `writetighter — deterministic and contextual prose revision for technical documentation

USAGE
  writetighter <command> [flags] [paths...]

COMMANDS
  lint      Run deterministic profile rules (no model access)
  revise    Run contextual model-based revision (requires model config)
  prompt    Export core and kind-specific revision guidance
  config    Show model configuration or run the setup wizard
  explain   Explain a lint rule
  profile   Manage profiles (install, list, verify)
  version   Print version and embedded profile information

Run "writetighter <command> --help" for command-specific flags and examples.

EXIT CODES
  0  command completed successfully
  1  lint findings reached --fail-on threshold
  2  usage, configuration, profile, or input failure
  3  model call or model response failure
`

const lintHelp = `writetighter lint — run deterministic profile rules

Checks input against the active profile's static rules. No model access is needed.

USAGE
  writetighter lint [flags] [paths...]

FLAGS
  --stdin              Read input from stdin
  --text <string>     Lint the provided text directly
  --kind <kind>       Document kind (default: description)
                      Choices: description, procedure, pr, code-comment, reference,
                      decision, incident, agent-instruction, status-update
  --profile <spec>    Profile id@version (default: discovered or user config)
  --config <path>     Path to .writetighter.toml (default: auto-discovered)
  --format <format>   Output format: human, json, agent (default: human)
  --fail-on <level>   Fail when findings reach this severity
                      Choices: none, warning, error (default: none)

INPUT
  Provide input via file paths, --stdin, or --text (mutually exclusive).

EXAMPLES
  writetighter lint README.md
  writetighter lint --stdin < README.md
  writetighter lint --text "Short text." --kind procedure
  writetighter lint --format json docs/*.md --fail-on warning
`

const reviseHelp = `writetighter revise — run contextual model-based revision

Sends input to an OpenAI-compatible model with revision guidance and returns
rewritten prose. Requires [llm] configuration in the user config.

USAGE
  writetighter revise [flags] [paths...]

FLAGS
  --stdin              Read input from stdin
  --text <string>     Revise the provided text directly
  --kind <kind>       Document kind (default: description)
                      Choices: description, procedure, pr, code-comment, reference,
                      decision, incident, agent-instruction, status-update
  --profile <spec>    Profile id@version (default: discovered or user config)
  --config <path>     Path to .writetighter.toml (default: auto-discovered)
  --format <format>   Output format: json, human (default: json)

INPUT
  Provide input via file paths, --stdin, or --text (mutually exclusive).

CONFIGURATION
  Revise requires [llm] model settings. Run "writetighter config" to configure.
  If config is missing and stdin is a terminal, interactive setup is offered
  automatically; otherwise the command exits with a hint.

EXAMPLES
  writetighter revise --text "Short text." --kind procedure
  writetighter revise --stdin < README.md
  writetighter revise docs/*.md --format human
`

const promptHelp = `writetighter prompt — export revision guidance

Prints the core and kind-specific revision guidance used by "writetighter revise".
No model access or configuration is needed.

USAGE
  writetighter prompt [--kind <kind>] [--format <format>]

FLAGS
  --kind <kind>        Document kind (default: description)
                      Choices: description, procedure, pr, code-comment, reference,
                      decision, incident, agent-instruction, status-update
  --format <format>   Output format: human, json (default: human)

EXAMPLES
  writetighter prompt
  writetighter prompt --kind code-comment
  writetighter prompt --kind pr --format json
`

const configHelp = `writetighter config — show or reconfigure model settings

When already configured, prints the current configuration with the API key
redacted. To reconfigure interactively, pass --wizard.

If no configuration exists yet, the interactive wizard runs automatically.

USAGE
  writetighter config [--wizard]

FLAGS
  --wizard, -w    Run the interactive configuration wizard

EXAMPLES
  writetighter config              # show sanitized config (or run wizard if unconfigured)
  writetighter config --wizard     # force the wizard even if already configured
`

const explainHelp = `writetighter explain — explain a lint rule

Prints information about a named lint rule from the active profile.

USAGE
  writetighter explain <rule-id> [--profile <spec>] [--format <format>]

FLAGS
  --profile <spec>    Profile id@version (default: discovered or user config)
  --format <format>   Output format: human, json (default: human)

EXAMPLES
  writetighter explain CORE.SENTENCE_LENGTH
  writetighter explain CORE.SENTENCE_LENGTH --format json
`

const profileHelp = `writetighter profile — manage profiles

USAGE
  writetighter profile <subcommand> [options]

SUBCOMMANDS
  install <path>    Install a profile bundle from a directory
  list              List embedded and installed profiles
  verify <spec>     Verify a profile or bundle

FLAGS (list, verify)
  --format <format>   Output format: human, json (default: human)

EXAMPLES
  writetighter profile list
  writetighter profile list --format json
  writetighter profile verify core@1
  writetighter profile install ./my-profile
`

const versionHelp = `writetighter version — print version information

USAGE
  writetighter version --json

FLAGS
  --json    Output version information as JSON

The output includes the tool version, build commit, and embedded profile details.
`

var helpTexts = map[string]string{
	"":        mainHelp,
	"lint":    lintHelp,
	"revise":  reviseHelp,
	"prompt":  promptHelp,
	"config":  configHelp,
	"explain": explainHelp,
	"profile": profileHelp,
	"version": versionHelp,
}
