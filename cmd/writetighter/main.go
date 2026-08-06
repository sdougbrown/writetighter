package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sdougbrown/writetighter/internal/app"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/setup"
	"golang.org/x/term"
)

func main() { os.Exit(run(os.Args[1:])) }

// loadEmbedded provides a package-level hook so tests can replace the
// embedded profile loader to simulate a no-profile state.
var loadEmbedded = profile.LoadEmbedded

type pathsFlag []string

func (f *pathsFlag) String() string { return strings.Join(*f, ",") }
func (f *pathsFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

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
	baseline := fs.String("git-compare", "", "")
	fs.Usage = func() { printHelp("lint") }
	if err := fs.Parse(normalizeInterspersedFlags(args)); err != nil {
		return 2
	}
	params := app.LintParams{Paths: fs.Args(), Stdin: *stdin, Kind: *kind, Profile: *profile, ConfigPath: *configPath, Format: *format, FailOn: *failOn, GitCompare: *baseline}
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
	var referencePaths pathsFlag
	fs.Var(&referencePaths, "reference", "")
	fs.Usage = func() { printHelp("revise") }
	if err := fs.Parse(normalizeInterspersedFlags(args)); err != nil {
		return 2
	}
	params := app.ReviseParams{Paths: fs.Args(), Stdin: *stdin, Kind: *kind, Profile: *profile, ConfigPath: *configPath, Format: *format, ReferencePaths: []string(referencePaths)}
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
	contextTokens := fs.Int("context", 0, "Set model context window tokens")
	outputTokens := fs.Int("output-tokens", 0, "Set model max output tokens")
	model := fs.String("model", "", "Set model ID")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "config does not accept positional arguments")
		fmt.Fprintln(os.Stderr, "  Run `writetighter config --help` for usage.")
		return 2
	}

	hasTargetedFlags := *contextTokens > 0 || *outputTokens > 0 || *model != ""

	// Targeted flags and --wizard are mutually exclusive.
	if hasTargetedFlags && *wizard {
		fmt.Fprintln(os.Stderr, "--context, --output-tokens, and --model cannot be combined with --wizard")
		fmt.Fprintln(os.Stderr, "  Run `writetighter config --help` for usage.")
		return 2
	}

	// Targeted update path.
	if hasTargetedFlags {
		// Validate basic token relationship.
		if *contextTokens > 0 && *outputTokens > 0 && *outputTokens >= *contextTokens {
			fmt.Fprintf(os.Stderr, "--output-tokens (%d) must be less than --context (%d)\n", *outputTokens, *contextTokens)
			return 2
		}

		// Load existing config. Targeted flags require a valid existing config.
		existing, err := config.LoadUserConfig()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				usageErr("no existing config; run 'writetighter config --wizard' first")
				return 2
			}
			usageErr("failed to load config: " + err.Error())
			return 2
		}
		if existing.LLM.BaseURL == "" {
			usageErr("config is missing base_url; run 'writetighter config --wizard' first")
			return 2
		}

		// Save previous model for context-clearing logic.
		prevModel := existing.LLM.Model

		// If --model is specified, preflight it against the endpoint.
		if *model != "" {
			apiKey := resolveAPIKey(existing)
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			client := &http.Client{Timeout: 45 * time.Second}

			models, discErr := setup.ListModels(ctx, client, existing.LLM.BaseURL, apiKey)
			var found *setup.ModelInfo
			if discErr != nil || len(models) == 0 {
				// Discovery unavailable or empty: permit the explicitly entered model
				// ID and rely on chat preflight below. No capacity suggestion is
				// available in this case.
				if discErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: model discovery unavailable: %v; proceeding with explicit model %q (no capacity suggestion).\n", discErr, *model)
				} else {
					fmt.Fprintf(os.Stderr, "Warning: model discovery returned no models; proceeding with explicit model %q (no capacity suggestion).\n", *model)
				}
			} else {
				for i := range models {
					if models[i].ID == *model {
						found = &models[i]
						break
					}
				}
				if found == nil {
					usageErr(fmt.Sprintf("model %q was not reported by the endpoint", *model))
					return 2
				}
			}

			// If the model is actually changing, clear context window unless --context is also set.
			if *model != prevModel && *contextTokens == 0 {
				existing.LLM.ContextWindowTokens = 0
				existing.LLM.ContextWindowModel = ""
				fmt.Fprintf(os.Stderr, "Warning: model changed from %q to %q; context_window_tokens cleared.\n", prevModel, *model)
				fmt.Fprintf(os.Stderr, "  Run 'writetighter config --context N' to set the context window for the new model.\n")
			}

			existing.LLM.Model = *model
			// context_window_model records the model for which context_window_tokens
			// was last confirmed; only set it when a confirmed value exists.
			if existing.LLM.ContextWindowTokens > 0 || *contextTokens > 0 {
				existing.LLM.ContextWindowModel = *model
			}

			// Refresh response mode.
			newMode, probeErr := setup.ProbeResponseMode(ctx, client, existing.LLM.BaseURL, *model, apiKey)
			if probeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: response mode preflight failed for %q: %v\n", *model, probeErr)
			} else {
				existing.LLM.ResponseMode = newMode
				fmt.Fprintf(os.Stderr, "Response mode refreshed to %q for model %q.\n", newMode, *model)
			}

			// If metadata has a suggestion and no --context was given, show it.
			if found != nil && found.SuggestedContextWindow() > 0 && *contextTokens == 0 && existing.LLM.ContextWindowTokens == 0 {
				fmt.Fprintf(os.Stderr, "Model %q suggests a context window of %d tokens.\n", *model, found.SuggestedContextWindow())
				fmt.Fprintf(os.Stderr, "  Run 'writetighter config --context %d' to set it.\n", found.SuggestedContextWindow())
			}
		}

		// Apply --context (associate with current model).
		if *contextTokens > 0 {
			existing.LLM.ContextWindowTokens = *contextTokens
			if existing.LLM.Model != "" {
				existing.LLM.ContextWindowModel = existing.LLM.Model
			}
		}

		// Apply --output-tokens.
		if *outputTokens > 0 {
			existing.LLM.MaxOutputTokens = *outputTokens
		}

		// Final validation: if both are set, ensure output < context.
		if existing.LLM.ContextWindowTokens > 0 && existing.LLM.MaxOutputTokens > 0 &&
			existing.LLM.MaxOutputTokens >= existing.LLM.ContextWindowTokens {
			usageErr(fmt.Sprintf("max_output_tokens (%d) must be less than context_window_tokens (%d); not saved",
				existing.LLM.MaxOutputTokens, existing.LLM.ContextWindowTokens))
			return 2
		}

		// Save back.
		path, err := config.WriteUserConfig(existing)
		if err != nil {
			usageErr("failed to save config: " + err.Error())
			return 2
		}

		// Show sanitized result.
		tomlStr, err := existing.SanitizedTOML()
		if err != nil {
			usageErr("failed to render config: " + err.Error())
			return 2
		}
		fmt.Print(tomlStr)
		fmt.Fprintf(os.Stderr, "\nWrote %s\n", path)
		return 0
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

func resolveAPIKey(cfg *config.UserConfig) string {
	if cfg.LLM.APIKey != "" {
		return cfg.LLM.APIKey
	}
	if cfg.LLM.APIKeyEnv != "" {
		return os.Getenv(cfg.LLM.APIKeyEnv)
	}
	return ""
}

func stdinIsTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// The documented CLI permits paths before flags. flag.FlagSet stops at the first
// positional argument, so normalize the small lint/revise grammar before parsing.
func normalizeInterspersedFlags(args []string) []string {
	withValue := map[string]bool{"--kind": true, "--profile": true, "--config": true, "--format": true, "--fail-on": true, "--text": true, "--reference": true, "--git-compare": true}
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
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "Run `writetighter version --help` for usage.")
		return 2
	}
	r, _ := loadEmbedded()
	profiles := []any{}
	if r != nil {
		profiles = append(profiles, map[string]any{"id": string(r.ID), "version": string(r.Version), "sha256": r.SHA256})
	}
	if !*jsonFlag {
		fmt.Fprintf(os.Stdout, "writetighter %s", app.Version)
		if app.Commit != "" && app.Commit != "unknown" {
			fmt.Fprintf(os.Stdout, " (commit %s)", app.Commit)
		}
		if r != nil {
			fmt.Fprintf(os.Stdout, "\nembedded profile: %s@%s", r.ID, r.Version)
		}
		fmt.Fprintln(os.Stdout)
		return 0
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
  --git-compare <rev> Git revision to compare against for unfamiliar-term detection
                      Only valid with file paths. When set, flags terms that
                      appear in the changed prose but have no precedent in the
                      comparison revision. Advisory only (candidate/info).

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
  --reference <path>  Path to reference file or directory (repeatable)
                      Reference content provides broader context for revision
                      decisions. May be combined with paths, --stdin, or --text.

INPUT
  Provide input via file paths, --stdin, or --text (mutually exclusive).

CONFIGURATION
  Revise requires [llm] model settings. Run "writetighter config" to configure.
  If config is missing and stdin is a terminal, interactive setup is offered
  automatically; otherwise the command exits with a hint.

  Reference revision additionally requires context_window_tokens and
  max_output_tokens in the LLM configuration. Run:
    writetighter config --context TOKENS --output-tokens TOKENS
  to set them after initial configuration.

EXAMPLES
  writetighter revise --text "Short text." --kind procedure
  writetighter revise --stdin < README.md
  writetighter revise docs/*.md --format human
  writetighter revise docs/*.md --reference style-guide.md
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
  writetighter config [flags]

FLAGS
  --context <tokens>          Set model context window tokens
  --output-tokens <tokens>    Set model max output tokens
  --model <model>             Set model ID (re-preflights if possible)
  --wizard, -w                Run the interactive configuration wizard

NOTE
  --context, --output-tokens, and --model are mutually exclusive with --wizard.
  When any of these flags are given, the config file is updated directly without
  the interactive wizard.

EXAMPLES
  writetighter config                         # show sanitized config (or run wizard if unconfigured)
  writetighter config --wizard                # force the wizard even if already configured
  writetighter config --context 8192           # set context window to 8192 tokens
  writetighter config --output-tokens 4096     # set max output tokens to 4096
  writetighter config --model gemma4           # set context window model
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
  writetighter version [--json]

FLAGS
  --json    Output version information as JSON (default: human-readable)

Without --json, prints a compact human-readable summary. With --json, the
output includes the tool version, build commit, and embedded profile details.
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
