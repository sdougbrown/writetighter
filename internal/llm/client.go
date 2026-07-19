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
)

const (
	DefaultTimeout = 45 * time.Second
	MaxInputChars  = 32000
	MaxSuggestions = 20
	MaxOutputChars = 10000
	chatPath       = "/v1/chat/completions"
)

type Config struct {
	BaseURL      string
	Model        string
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
	Type       string     `json:"type"`
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
	apiKeyEnv  string
	model      string
	mode       string
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Model == "" {
		return nil, errors.New("llm model required")
	}
	endpoint, err := normalizeEndpoint(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &Client{httpClient: &http.Client{Timeout: cfg.Timeout}, endpoint: endpoint, apiKeyEnv: cfg.APIKeyEnv, model: cfg.Model, mode: cfg.ResponseMode}, nil
}

func (c *Client) Endpoint() string { return c.endpoint }

func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("llm request requires messages")
	}
	for _, m := range req.Messages {
		if len(m.Content) > MaxInputChars {
			return nil, fmt.Errorf("llm input too large")
		}
	}
	req.Model = c.model
	if req.ResponseFormat == nil && c.mode != "" && c.mode != "auto" {
		req.ResponseFormat = buildResponseFormat(c.mode)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKeyEnv != "" {
		if key := os.Getenv(c.apiKeyEnv); key != "" {
			httpReq.Header.Set("Authorization", "Bearer "+key)
		}
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out Response
	dec := json.NewDecoder(io.LimitReader(resp.Body, MaxOutputChars))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func buildResponseFormat(mode string) *ResponseFormat {
	rf := &ResponseFormat{Type: mode}
	if mode == "json_schema" {
		schema := `{"type":"object","properties":{"findings":{"type":"array","items":{"$ref":"#/$defs/AdvisorFinding"}},"$defs":{"AdvisorFinding":{"type":"object","properties":{"source_range":{"type":"object","properties":{"start":{"type":"integer"},"end":{"type":"integer"}},"required":["start","end"]},"rule_ids":{"type":"array","items":{"type":"string"}},"reason":{"type":"string"},"replacement":{"type":"string"},"confidence":{"type":"number"}},"required":["source_range","rule_ids","reason"]}}},"required":["findings"]}`
		rf.JSONSchema = &JSONSchema{
			Name:   "advisor_response",
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
	u.Path = chatPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
