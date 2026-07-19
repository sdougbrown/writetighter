package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sdougbrown/writetighter/internal/app"
	"github.com/sdougbrown/writetighter/internal/profile"
)

func main() { os.Exit(run(os.Args[1:])) }

// Exit codes:
// 0: check completed and the selected failure threshold was not reached.
// 1: check completed and findings reached --fail-on.
// 2: usage, configuration, profile, input, or Stage 1 stub failures.
// 3: --require-llm was requested and the LLM stage failed or was skipped.
func run(args []string) int {
	if len(args) == 0 {
		usageErr("usage: no input specified")
		return 2
	}
	switch args[0] {
	case "version":
		return runVersion(args[1:])
	case "check":
		return runCheck(args[1:])
	case "explain":
		return runExplain(args[1:])
	case "profile":
		return runProfile(args[1:])
	default:
		usageErr("unknown command")
		return 2
	}
}

func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	stdin := fs.Bool("stdin", false, "")
	kind := fs.String("kind", "description", "")
	profile := fs.String("profile", "", "")
	configPath := fs.String("config", "", "")
	format := fs.String("format", "human", "")
	llm := fs.Bool("llm", false, "")
	requireLLM := fs.Bool("require-llm", false, "")
	baseURL := fs.String("llm-base-url", "", "")
	model := fs.String("llm-model", "", "")
	responseMode := fs.String("llm-response-mode", "", "")
	failOn := fs.String("fail-on", "none", "")
	fs.Usage = func() {}
	if err := fs.Parse(normalizeInterspersedFlags(args)); err != nil {
		return 2
	}
	params := app.CheckParams{Paths: fs.Args(), Stdin: *stdin, Kind: *kind, Profile: *profile, ConfigPath: *configPath, Format: *format, LLM: *llm, RequireLLM: *requireLLM, LLMBaseURL: *baseURL, LLMModel: *model, LLMResponseMode: *responseMode, FailOn: *failOn}
	if params.Stdin && len(params.Paths) > 0 {
		usageErr("usage: mutually exclusive arguments")
		return 2
	}
	if !params.Stdin && len(params.Paths) == 0 {
		usageErr("usage: no input specified")
		return 2
	}
	if params.RequireLLM && !params.LLM {
		usageErr("usage: --require-llm requires --llm")
		return 2
	}
	err := app.New().RunCheck(params)
	switch {
	case errors.Is(err, app.ErrRequireLLM):
		return 3
	case errors.Is(err, app.ErrFailThreshold):
		return 1
	case err != nil:
		usageErr(err.Error())
		return 2
	}
	return 0
}

// The documented CLI permits paths before flags. flag.FlagSet stops at the first
// positional argument, so normalize the small check grammar before parsing.
func normalizeInterspersedFlags(args []string) []string {
	withValue := map[string]bool{"--kind": true, "--profile": true, "--config": true, "--format": true, "--llm-base-url": true, "--llm-model": true, "--llm-response-mode": true, "--fail-on": true}
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
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {}
	profile := fs.String("profile", "", "")
	format := fs.String("format", "human", "")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 1 || (*format != "human" && *format != "json") {
		usageErr("invalid explain arguments")
		return 2
	}
	if err := app.New().RunExplainWithOptions(fs.Args()[0], *profile, *format); err != nil {
		usageErr(err.Error())
		return 2
	}
	return 0
}

func runProfile(args []string) int {
	if len(args) == 0 {
		usageErr("profile subcommand required")
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
		usageErr("unknown profile subcommand")
		return 2
	}
}

func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "")
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil || !*jsonFlag || len(fs.Args()) != 0 {
		usageErr("version requires --json")
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
