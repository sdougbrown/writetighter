package main

import (
	"encoding/json"
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
		usageErr("not implemented")
		return 2
	}
}

func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	stdin := fs.Bool("stdin", false, "")
	kind := fs.String("kind", "", "")
	profile := fs.String("profile", "", "")
	configPath := fs.String("config", "", "")
	format := fs.String("format", "human", "")
	llm := fs.Bool("llm", false, "")
	requireLLM := fs.Bool("require-llm", false, "")
	baseURL := fs.String("llm-base-url", "", "")
	model := fs.String("llm-model", "", "")
	responseMode := fs.String("llm-response-mode", "", "")
	failOn := fs.String("fail-on", "", "")
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
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
	if err := app.New().RunCheck(params); err != nil {
		usageErr(err.Error())
		return 2
	}
	return 0
}

func runExplain(args []string) int {
	profile := ""
	format := "human"
	ruleID := ""

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--profile" && i+1 < len(args):
			profile = args[i+1]
			i++
		case args[i] == "--format" && i+1 < len(args):
			format = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--"):
			// unknown flag, skip
		default:
			ruleID = args[i]
		}
	}

	if ruleID == "" {
		usageErr("not implemented")
		return 2
	}
	if err := app.New().RunExplainWithOptions(ruleID, profile, format); err != nil {
		usageErr(err.Error())
		return 2
	}
	return 0
}

func runProfile(args []string) int {
	if len(args) == 0 {
		usageErr("not implemented")
		return 2
	}
	switch args[0] {
	case "install":
		if len(args) != 2 || args[1] == "" {
			usageErr("not implemented")
			return 2
		}
		if err := app.New().RunProfileInstall(args[1]); err != nil {
			usageErr(err.Error())
			return 2
		}
		return 0
	case "list":
		if err := app.New().RunProfileList(); err != nil {
			usageErr(err.Error())
			return 2
		}
		return 0
	case "verify":
		if len(args) != 2 || args[1] == "" {
			usageErr("not implemented")
			return 2
		}
		if err := app.New().RunProfileVerify(args[1]); err != nil {
			usageErr(err.Error())
			return 2
		}
		return 0
	default:
		usageErr("not implemented")
		return 2
	}
}

func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "")
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil || !*jsonFlag {
		usageErr("not implemented")
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

func usageErr(msg string) { fmt.Fprintln(os.Stderr, msg) }
