package main

import (
	"fmt"
	"os"

	"github.com/sdougbrown/writetighter/internal/cli"
)

func main() { os.Exit(run(os.Args[1:])) }

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
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
	case "explain":
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
	case "profile":
		// Migrated to the clicontract-declared command surface.
		return cli.Run(args, os.Stdout, os.Stderr)
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
	case "version", "prompt", "lint", "revise", "rewrite", "explain", "profile", "config":
		// Migrated command: its help renders from the declaration.
		return cli.Run([]string{args[0], "--help"}, os.Stdout, os.Stderr)
	}
	printHelp(args[0])
	return cli.ExitOK
}

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

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

func printHelp(command string) {
	text, ok := helpTexts[command]
	if !ok {
		fmt.Fprintf(os.Stdout, "no help available for %q\n\n", command)
		text = helpTexts[""]
	}
	fmt.Fprint(os.Stdout, text)
}

var helpTexts = map[string]string{
	"": mainHelp,
}
