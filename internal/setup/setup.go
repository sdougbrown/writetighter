package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/llm"
)

const (
	defaultKeyEnv      = "WRITETIGHTER_API_KEY"
	defaultTimeout     = config.DefaultLLMTimeout
	defaultMaxRequests = 32
)

// StdinIsTerminal reports whether stdin is an interactive terminal.
func StdinIsTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// RunInteractive runs the interactive configuration wizard against the
// process's stdio. Secure API key entry requires a terminal.
func RunInteractive(ctx context.Context) error {
	_, err := Run(ctx, Options{
		In:  os.Stdin,
		Out: os.Stderr,
		ReadSecret: func(prompt string) (string, error) {
			if !StdinIsTerminal() {
				return "", errors.New("secure API key input requires an interactive terminal")
			}
			fmt.Fprint(os.Stderr, prompt)
			secret, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			return string(secret), err
		},
	})
	return err
}

// SecretReader reads a secret without persisting it.
type SecretReader func(prompt string) (string, error)

// Options controls the interactive configuration workflow.
type Options struct {
	In         io.Reader
	Out        io.Writer
	ReadSecret SecretReader
	HTTPClient *http.Client
}

// Result describes the generated configuration.
type Result struct {
	Path             string
	BaseURL          string
	Model            string
	ResponseMode     string
	APIKeyEnv        string
	NeedsEnvironment bool
}

// Run discovers an OpenAI-compatible endpoint and atomically writes user config.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.In == nil || opts.Out == nil {
		return nil, errors.New("configuration input and output are required")
	}
	if opts.ReadSecret == nil {
		opts.ReadSecret = func(string) (string, error) { return "", errors.New("secure secret input is unavailable") }
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}

	wizard := &wizard{reader: bufio.NewReader(opts.In), out: opts.Out}
	existing, err := config.LoadUserConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(opts.Out, "The existing user config is invalid and cannot be reused.")
		replace, askErr := wizard.ask("Replace it? (yes/no)", "no")
		if askErr != nil {
			return nil, askErr
		}
		replace = strings.ToLower(replace)
		if replace != "yes" && replace != "y" {
			return nil, errors.New("existing user config was not replaced")
		}
		existing = &config.UserConfig{}
	}
	if existing == nil {
		existing = &config.UserConfig{}
	}

	fmt.Fprintln(opts.Out, "WriteTighter model configuration")
	fmt.Fprintln(opts.Out, "Choose whether a key is omitted, stored in the private user config, or read from an environment variable.")

	endpointDefault := existing.LLM.BaseURL
	endpointInput, err := wizard.ask("OpenAI-compatible API URL, localhost port, or host:port", endpointDefault)
	if err != nil {
		return nil, err
	}
	baseURL, err := NormalizeBaseURL(endpointInput)
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(opts.Out, "  1) No API key")
	fmt.Fprintln(opts.Out, "  2) Save API key in config.toml (0600)")
	fmt.Fprintln(opts.Out, "  3) Read API key from an environment variable")
	authDefault := "1"
	if existing.LLM.APIKey != "" {
		authDefault = "2"
	} else if existing.LLM.APIKeyEnv != "" {
		authDefault = "3"
	}
	authChoice, err := wizard.ask("Authentication", authDefault)
	if err != nil {
		return nil, err
	}
	apiKeyEnv := ""
	apiKey := ""
	storedAPIKey := ""
	needsEnvironment := false
	switch authChoice {
	case "1":
	case "2":
		secretPrompt := "API key to save in private config: "
		if existing.LLM.APIKey != "" {
			secretPrompt = "API key to save (leave blank to keep current): "
		}
		apiKey, err = opts.ReadSecret(secretPrompt)
		if err != nil {
			return nil, err
		}
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			apiKey = existing.LLM.APIKey
		}
		if apiKey == "" {
			return nil, errors.New("API key is required")
		}
		storedAPIKey = apiKey
	case "3":
		envDefault := existing.LLM.APIKeyEnv
		if envDefault == "" {
			envDefault = defaultKeyEnv
		}
		apiKeyEnv, err = wizard.ask("Environment variable name for the API key", envDefault)
		if err != nil {
			return nil, err
		}
		if !validEnvName(apiKeyEnv) {
			return nil, fmt.Errorf("invalid API key environment variable name %q", apiKeyEnv)
		}
		apiKey = os.Getenv(apiKeyEnv)
		if apiKey == "" {
			apiKey, err = opts.ReadSecret("API key (preflight only; not saved): ")
			if err != nil {
				return nil, err
			}
			apiKey = strings.TrimSpace(apiKey)
			if apiKey == "" {
				return nil, errors.New("API key is required for preflight")
			}
			if err := os.Setenv(apiKeyEnv, apiKey); err != nil {
				return nil, err
			}
			needsEnvironment = true
		}
	default:
		return nil, fmt.Errorf("authentication selection must be 1, 2, or 3")
	}

	fmt.Fprintf(opts.Out, "Querying %s/models ...\n", baseURL)
	models, discoveryErr := llm.ListModels(baseURL, apiKey, defaultTimeout)
	var model string
	var selectedModelInfo *llm.ModelInfo
	if discoveryErr != nil || len(models) == 0 {
		if discoveryErr != nil {
			fmt.Fprintf(opts.Out, "Model discovery was unavailable: %v\n", discoveryErr)
		} else {
			fmt.Fprintln(opts.Out, "Model discovery returned no model IDs.")
		}
		model, err = wizard.ask("Model ID", existing.LLM.Model)
	} else {
		model, selectedModelInfo, err = wizard.selectModel(models, existing.LLM.Model)
	}
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(opts.Out, "Preflighting model %s ...\n", model)
	// Probe from most constrained to least; stop at the first mode the
	// endpoint accepts. The order lives with the probe implementation.
	responseMode, preflightErr := llm.ProbeResponseModeWithProgress(ctx, opts.HTTPClient, baseURL, model, apiKey, func(mode string, modeErr error, next string) {
		fmt.Fprintf(opts.Out, "%s was not accepted (%v); trying %s.\n", mode, modeErr, next)
	})
	if preflightErr != nil {
		return nil, fmt.Errorf("model preflight failed: %w", preflightErr)
	}

	// Inform the user about auto-detected context capacity. The wizard does
	// not persist context window tokens — revise auto-detects from the
	// /v1/models endpoint at runtime, or the user can override with
	// --context-tokens at revise time.
	if selectedModelInfo != nil {
		if cw := selectedModelInfo.SuggestedContextWindow(); cw > 0 {
			fmt.Fprintf(opts.Out, "Model %s reports a context window of %d tokens.\n", model, cw)
			fmt.Fprintln(opts.Out, "Context capacity will be auto-detected by 'revise' when needed.")
		}
	}

	existing.LLM = config.LLMConfig{
		Provider:     "openai-compatible",
		BaseURL:      baseURL,
		Model:        model,
		APIKey:       storedAPIKey,
		APIKeyEnv:    apiKeyEnv,
		Timeout:      defaultTimeout.String(),
		ResponseMode: responseMode,
		MaxRequests:  defaultMaxRequests,
		CodeModel:    existing.LLM.CodeModel,
	}
	path, err := config.WriteUserConfig(existing)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(opts.Out, "Wrote %s with mode %s.\n", path, responseMode)
	if needsEnvironment {
		fmt.Fprintf(opts.Out, "Before running revise, set %s in your shell environment.\n", apiKeyEnv)
	}
	return &Result{Path: path, BaseURL: baseURL, Model: model, ResponseMode: responseMode, APIKeyEnv: apiKeyEnv, NeedsEnvironment: needsEnvironment}, nil
}

type wizard struct {
	reader *bufio.Reader
	out    io.Writer
}

func (w *wizard) ask(label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(w.out, "%s: ", label)
	} else {
		fmt.Fprintf(w.out, "%s [%s]: ", label, defaultValue)
	}
	line, err := w.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = defaultValue
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}

func (w *wizard) selectModel(models []llm.ModelInfo, previous string) (string, *llm.ModelInfo, error) {
	defaultIndex := 0
	for i, model := range models {
		if model.ID == previous {
			defaultIndex = i
		}
		fmt.Fprintf(w.out, "  %d) %s\n", i+1, model.ID)
	}
	choice, err := w.ask("Select a model by number or ID", strconv.Itoa(defaultIndex+1))
	if err != nil {
		return "", nil, err
	}
	if n, err := strconv.Atoi(choice); err == nil {
		if n < 1 || n > len(models) {
			return "", nil, fmt.Errorf("model selection %d is out of range", n)
		}
		info := models[n-1]
		return info.ID, &info, nil
	}
	for _, model := range models {
		if choice == model.ID {
			info := model
			return info.ID, &info, nil
		}
	}
	return "", nil, fmt.Errorf("model %q was not reported by the endpoint", choice)
}

// NormalizeBaseURL accepts a URL, a localhost port, or host:port and returns an API root.
func NormalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("API URL is required")
	}
	if port, err := strconv.Atoi(value); err == nil {
		if port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid localhost port %d", port)
		}
		value = fmt.Sprintf("http://localhost:%d/v1", port)
	} else if !strings.Contains(value, "://") {
		if _, _, err := net.SplitHostPort(value); err == nil {
			value = "http://" + value + "/v1"
		} else {
			value = "https://" + value
		}
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("API URL must use http or https")
	}
	if u.Host == "" || u.User != nil {
		return "", errors.New("API URL must contain a host and no embedded credentials")
	}
	path := strings.TrimRight(u.Path, "/")
	path = strings.TrimSuffix(path, "/chat/completions")
	path = strings.TrimSuffix(path, "/models")
	if path == "" {
		path = "/v1"
	}
	u.Path = strings.TrimRight(path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func validEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
