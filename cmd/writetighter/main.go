package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/sdougbrown/writetighter/internal/app"
	"github.com/sdougbrown/writetighter/internal/cli"
	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/llm"
	"github.com/sdougbrown/writetighter/internal/setup"
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
		return cli.ExitOK
	}
	switch args[0] {
	case "help":
		return runHelp(args[1:])
	case "version":
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
	case "lint":
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
	case "revise":
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
	case "rewrite":
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
	case "prompt":
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
	case "config":
		return runConfig(args[1:])
	case "explain":
		return runExplain(args[1:])
	case "profile":
		return runProfile(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printHelp("")
		return cli.ExitUsage
	}
}

func runHelp(args []string) int {
	if len(args) == 0 {
		printHelp("")
		return cli.ExitOK
	}
	switch args[0] {
	case "version", "prompt", "lint", "revise", "rewrite":
		// Migrated command: its help renders from the declaration.
		return cli.Run([]string{args[0], "--help"}, os.Stdout, os.Stderr)
	}
	printHelp(args[0])
	return cli.ExitOK
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

func runConfig(args []string) int {
	if wantsHelp(args) {
		printHelp("config")
		return cli.ExitOK
	}
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { printHelp("config") }
	wizard := fs.Bool("wizard", false, "Run the interactive configuration wizard")
	fs.BoolVar(wizard, "w", false, "Run the interactive configuration wizard (short for --wizard)")
	model := fs.String("model", "", "Set model ID")
	codeModel := fs.String("code-model", "", "Set the model used for code-aware comment revision")
	if err := fs.Parse(args); err != nil {
		return cli.ExitUsage
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "config does not accept positional arguments")
		fmt.Fprintln(os.Stderr, "  Run `writetighter config --help` for usage.")
		return cli.ExitUsage
	}

	hasTargetedFlags := *model != "" || *codeModel != ""

	// Targeted flags and --wizard are mutually exclusive.
	if hasTargetedFlags && *wizard {
		fmt.Fprintln(os.Stderr, "--model and --code-model cannot be combined with --wizard")
		fmt.Fprintln(os.Stderr, "  Run `writetighter config --help` for usage.")
		return cli.ExitUsage
	}

	// Targeted update path.
	if hasTargetedFlags {
		// Load existing config. Targeted flags require a valid existing config.
		existing, err := config.LoadUserConfig()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				usageErr("no existing config; run 'writetighter config --wizard' first")
				return cli.ExitUsage
			}
			usageErr("failed to load config: " + err.Error())
			return cli.ExitUsage
		}
		if existing.LLM.BaseURL == "" {
			usageErr("config is missing base_url; run 'writetighter config --wizard' first")
			return cli.ExitUsage
		}

		// If --model is specified, preflight it against the endpoint.
		if *model != "" {
			apiKey := resolveAPIKey(existing)
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			models, discErr := llm.ListModels(existing.LLM.BaseURL, apiKey, 45*time.Second)
			if discErr != nil || len(models) == 0 {
				if discErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: model discovery unavailable: %v; proceeding with explicit model %q.\n", discErr, *model)
				} else {
					fmt.Fprintf(os.Stderr, "Warning: model discovery returned no models; proceeding with explicit model %q.\n", *model)
				}
			} else {
				found := false
				for i := range models {
					if models[i].ID == *model {
						found = true
						break
					}
				}
				if !found {
					usageErr(fmt.Sprintf("model %q was not reported by the endpoint", *model))
					return cli.ExitUsage
				}
			}

			existing.LLM.Model = *model

			// Refresh response mode.
			client := &http.Client{Timeout: 45 * time.Second}
			newMode, probeErr := setup.ProbeResponseMode(ctx, client, existing.LLM.BaseURL, *model, apiKey)
			if probeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: response mode preflight failed for %q: %v\n", *model, probeErr)
			} else {
				existing.LLM.ResponseMode = newMode
				fmt.Fprintf(os.Stderr, "Response mode refreshed to %q for model %q.\n", newMode, *model)
			}
		}

		// Apply --code-model.
		if *codeModel != "" {
			existing.LLM.CodeModel = *codeModel
		}

		// Save back.
		path, err := config.WriteUserConfig(existing)
		if err != nil {
			usageErr("failed to save config: " + err.Error())
			return cli.ExitUsage
		}

		// Show sanitized result.
		tomlStr, err := existing.SanitizedTOML()
		if err != nil {
			usageErr("failed to render config: " + err.Error())
			return cli.ExitUsage
		}
		fmt.Print(tomlStr)
		fmt.Fprintf(os.Stderr, "\nWrote %s\n", path)
		return cli.ExitOK
	}

	// If not explicitly requesting the wizard, check whether a model is configured.
	// When a model is set, show a sanitized view of the config and hint at --wizard.
	if !*wizard {
		existing, err := config.LoadUserConfig()
		if err == nil && existing != nil && existing.LLM.Model != "" {
			tomlStr, err := existing.SanitizedTOML()
			if err != nil {
				usageErr("failed to render config: " + err.Error())
				return cli.ExitUsage
			}
			fmt.Print(tomlStr)
			if existing.LLM.APIKey != "" {
				fmt.Fprintln(os.Stderr, "\n(API key is stored in config.toml but redacted here.)")
			}
			fmt.Fprintln(os.Stderr, "To reconfigure, run: writetighter config --wizard")
			return cli.ExitOK
		}
	}

	// Run the interactive wizard (either --wizard was passed, or not yet configured).
	if err := setup.RunInteractive(context.Background()); err != nil {
		usageErr("config failed: " + err.Error())
		return cli.ExitUsage
	}
	return cli.ExitOK
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

func runExplain(args []string) int {
	if wantsHelp(args) {
		printHelp("explain")
		return cli.ExitOK
	}
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { printHelp("explain") }
	profile := fs.String("profile", "", "")
	format := fs.String("format", "human", "")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 1 || (*format != "human" && *format != "json") {
		fmt.Fprintln(os.Stderr, "invalid explain arguments")
		fmt.Fprintln(os.Stderr, "  Run `writetighter explain --help` for usage.")
		return cli.ExitUsage
	}
	if err := app.New().RunExplainWithOptions(fs.Args()[0], *profile, *format); err != nil {
		usageErr(err.Error())
		return cli.ExitUsage
	}
	return cli.ExitOK
}

func runProfile(args []string) int {
	if wantsHelp(args) {
		printHelp("profile")
		return cli.ExitOK
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "profile subcommand required")
		fmt.Fprintln(os.Stderr, "  Run `writetighter profile --help` for usage.")
		return cli.ExitUsage
	}
	switch args[0] {
	case "install":
		if len(args) != 2 || args[1] == "" {
			usageErr("profile install requires one bundle path")
			return cli.ExitUsage
		}
		if err := app.New().RunProfileInstall(args[1]); err != nil {
			usageErr(err.Error())
			return cli.ExitUsage
		}
		return cli.ExitOK
	case "list":
		format, ok := parseProfileFormat(args[1:])
		if !ok {
			usageErr("invalid profile list arguments")
			return cli.ExitUsage
		}
		if err := app.New().RunProfileList(format); err != nil {
			usageErr(err.Error())
			return cli.ExitUsage
		}
		return cli.ExitOK
	case "verify":
		if len(args) < 2 || (len(args) >= 2 && args[1] == "") {
			usageErr("profile verify requires a profile or bundle path")
			return cli.ExitUsage
		}
		spec := args[1]
		format, ok := parseProfileFormat(args[2:])
		if !ok {
			usageErr("invalid profile verify arguments")
			return cli.ExitUsage
		}
		if err := app.New().RunProfileVerify(spec, format); err != nil {
			usageErr(err.Error())
			return cli.ExitUsage
		}
		return cli.ExitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown profile subcommand %q\n", args[0])
		printHelp("profile")
		return cli.ExitUsage
	}
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
  rewrite   Run whole-passage contextual rewrite (requires model config)
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

const configHelp = `writetighter config — show or reconfigure model settings

When already configured, prints the current configuration with the API key
redacted. To reconfigure interactively, pass --wizard.

If no configuration exists yet, the interactive wizard runs automatically.

USAGE
  writetighter config [flags]

FLAGS
  --model <model>             Set model ID (re-preflights if possible)
  --code-model <model>        Set the model used for code-aware comment revision
  --wizard, -w                Run the interactive configuration wizard

NOTE
  --model and --code-model are mutually exclusive with --wizard.
  When either flag is given, the config file is updated directly without
  the interactive wizard.

EXAMPLES
  writetighter config                         # show sanitized config (or run wizard if unconfigured)
  writetighter config --wizard                # force the wizard even if already configured
  writetighter config --model gemma4           # switch model (refreshes response mode)
  writetighter config --code-model qwen-coder  # set a separate model for code comment revision
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

var helpTexts = map[string]string{
	"":        mainHelp,
	"config":  configHelp,
	"explain": explainHelp,
	"profile": profileHelp,
}
