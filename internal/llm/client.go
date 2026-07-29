package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultTimeout   = 45 * time.Second
	MaxInputChars    = 32000
	MaxSuggestions   = 20
	MaxOutputChars   = 10000
	MaxEnvelopeChars = 64 * 1024
	chatPath         = "/chat/completions"
)

type Config struct {
	BaseURL      string
	Model        string
	APIKey       string
	APIKeyEnv    string
	Timeout      time.Duration
	ResponseMode string
}

type Request struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type Response struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

type Client struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	apiKeyEnv  string
	model      string
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Model == "" {
		return nil, errors.New("llm model required")
	}
	if cfg.APIKey != "" && cfg.APIKeyEnv != "" {
		return nil, errors.New("llm api key and api key environment variable are mutually exclusive")
	}
	endpoint, err := normalizeEndpoint(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &Client{httpClient: &http.Client{Timeout: cfg.Timeout}, endpoint: endpoint, apiKey: cfg.APIKey, apiKeyEnv: cfg.APIKeyEnv, model: cfg.Model}, nil
}

func (c *Client) Endpoint() string { return c.endpoint }

func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("llm request requires messages")
	}
	totalInput := 0
	for _, m := range req.Messages {
		totalInput += len(m.Content)
	}
	if totalInput > MaxInputChars {
		return nil, fmt.Errorf("llm input too large")
	}
	req.Model = c.model
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	key := c.apiKey
	if key == "" && c.apiKeyEnv != "" {
		key = os.Getenv(c.apiKeyEnv)
	}
	if key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		message := strings.TrimSpace(string(b))
		if key != "" {
			message = strings.ReplaceAll(message, key, "[REDACTED]")
		}
		message = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, message)
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, message)
	}
	envelope, err := io.ReadAll(io.LimitReader(resp.Body, MaxEnvelopeChars+1))
	if err != nil {
		return nil, err
	}
	if len(envelope) > MaxEnvelopeChars {
		return nil, errors.New("llm response envelope too large")
	}
	var out Response
	// OpenAI-compatible envelopes commonly include id, usage, model, and
	// timing fields. Only the assistant content is security-sensitive and is
	// validated strictly by the caller-specific response validator.
	if err := json.Unmarshal(envelope, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// buildReviseResponseFormat builds the structured revision schema.
func buildReviseResponseFormat(mode string) *ResponseFormat {
	if mode == "prompt_json" || mode == "auto" || mode == "" {
		return nil
	}
	rf := &ResponseFormat{Type: mode}
	if mode == "json_schema" {
		schema := `{"type":"object","additionalProperties":false,"properties":{"findings":{"type":"array","maxItems":20,"items":{"type":"object","additionalProperties":false,"properties":{"kind":{"type":"string","enum":["rewrite","clarification"]},"source_text":{"type":"string","minLength":1},"source_range":{"type":"object","additionalProperties":false,"properties":{"start":{"type":"integer"},"end":{"type":"integer"}},"required":["start","end"]},"principle_ids":{"type":"array","minItems":1,"items":{"type":"string","enum":["CORE.APPROVED_WORDS","CORE.ONE_TERM_IDEA","CORE.SHORT_SENTENCE","CORE.ACTIVE_DIRECT_VOICE","CORE.ONE_TOPIC_PARAGRAPH","CORE.EXPLICIT_RELATIONSHIPS"]}},"reason":{"type":"string","minLength":1},"replacement":{"type":["string","null"]},"question":{"type":["string","null"]},"confidence":{"type":"number","minimum":0,"maximum":1}},"required":["kind","source_text","source_range","principle_ids","reason","replacement","question","confidence"]}}},"required":["findings"]}`
		rf.JSONSchema = &JSONSchema{
			Name:   "revise_response",
			Schema: json.RawMessage(schema),
			Strict: true,
		}
	}
	return rf
}

func normalizeEndpoint(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("invalid llm base url scheme")
	}
	if u.Host == "" {
		return "", errors.New("invalid llm base url host")
	}
	if u.User != nil {
		return "", errors.New("llm base url must not contain credentials")
	}
	path := strings.TrimRight(u.Path, "/")
	path = strings.TrimSuffix(path, chatPath)
	if path == "" {
		path = "/v1"
	}
	u.Path = strings.TrimRight(path, "/") + chatPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
