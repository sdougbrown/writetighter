package setup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/config"
)

const (
	defaultKeyEnv      = "WRITETIGHTER_API_KEY"
	defaultTimeout     = 45 * time.Second
	defaultMaxRequests = 32
	maxResponse        = 1 << 20
)

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
	models, discoveryErr := listModels(ctx, opts.HTTPClient, baseURL, apiKey)
	var model string
	if discoveryErr != nil || len(models) == 0 {
		if discoveryErr != nil {
			fmt.Fprintf(opts.Out, "Model discovery was unavailable: %v\n", discoveryErr)
		} else {
			fmt.Fprintln(opts.Out, "Model discovery returned no model IDs.")
		}
		model, err = wizard.ask("Model ID", existing.LLM.Model)
	} else {
		model, err = wizard.selectModel(models, existing.LLM.Model)
	}
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(opts.Out, "Preflighting model %s ...\n", model)
	responseMode := "json_object"
	if err := probeModel(ctx, opts.HTTPClient, baseURL, model, apiKey, responseMode); err != nil {
		fmt.Fprintf(opts.Out, "json_object was not accepted (%v); trying prompt-only JSON.\n", err)
		responseMode = "prompt_json"
		if err := probeModel(ctx, opts.HTTPClient, baseURL, model, apiKey, responseMode); err != nil {
			return nil, fmt.Errorf("model preflight failed: %w", err)
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

func (w *wizard) selectModel(models []string, previous string) (string, error) {
	defaultIndex := 0
	for i, model := range models {
		if model == previous {
			defaultIndex = i
		}
		fmt.Fprintf(w.out, "  %d) %s\n", i+1, model)
	}
	choice, err := w.ask("Select a model by number or ID", strconv.Itoa(defaultIndex+1))
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(choice); err == nil {
		if n < 1 || n > len(models) {
			return "", fmt.Errorf("model selection %d is out of range", n)
		}
		return models[n-1], nil
	}
	for _, model := range models {
		if choice == model {
			return model, nil
		}
	}
	return "", fmt.Errorf("model %q was not reported by the endpoint", choice)
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

type modelsEnvelope struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func listModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]string, error) {
	body, err := doJSON(ctx, client, http.MethodGet, baseURL+"/models", apiKey, nil)
	if err != nil {
		return nil, err
	}
	var envelope modelsEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	seen := make(map[string]struct{})
	models := make([]string, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		id := strings.TrimSpace(item.ID)
		if !validModelID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	sort.Strings(models)
	return models, nil
}

func probeModel(ctx context.Context, client *http.Client, baseURL, model, apiKey, responseMode string) error {
	request := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "Return one JSON object with exactly this shape: {\"ok\":true}."},
			{"role": "user", "content": "Preflight the connection."},
		},
	}
	if responseMode == "json_object" {
		request["response_format"] = map[string]string{"type": "json_object"}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	body, err := doJSON(ctx, client, http.MethodPost, baseURL+"/chat/completions", apiKey, payload)
	if err != nil {
		return err
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode chat response: %w", err)
	}
	if len(envelope.Choices) == 0 || strings.TrimSpace(envelope.Choices[0].Message.Content) == "" {
		return errors.New("chat response contained no assistant content")
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &object); err != nil {
		return fmt.Errorf("assistant did not return JSON: %w", err)
	}
	return nil
}

func doJSON(ctx context.Context, client *http.Client, method, endpoint, apiKey string, payload []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponse {
		return nil, errors.New("preflight response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		message := strings.TrimSpace(string(body))
		if apiKey != "" {
			message = strings.ReplaceAll(message, apiKey, "[REDACTED]")
		}
		message = safeDisplay(message)
		return nil, fmt.Errorf("HTTP %d: %s", response.StatusCode, message)
	}
	return body, nil
}

func validModelID(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func safeDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
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
