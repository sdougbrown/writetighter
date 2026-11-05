package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// probeMaxResponse bounds a preflight response body.
const probeMaxResponse = 1 << 20

// ProbeResponseMode probes the model for supported response modes and returns
// the best available mode. It probes json_schema, json_object, then prompt_json.
func ProbeResponseMode(ctx context.Context, client *http.Client, baseURL, model, apiKey string) (string, error) {
	probeOrder := []struct {
		mode      string
		nextLabel string
	}{
		{"json_schema", "json_object"},
		{"json_object", "prompt-only JSON"},
		{"prompt_json", ""},
	}
	for i, step := range probeOrder {
		if err := probeMode(ctx, client, baseURL, model, apiKey, step.mode); err != nil {
			if i == len(probeOrder)-1 {
				return "", err
			}
			continue
		}
		return step.mode, nil
	}
	return "", errors.New("no response mode accepted")
}

func probeMode(ctx context.Context, client *http.Client, baseURL, model, apiKey, responseMode string) error {
	request := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "Return one JSON object with exactly this shape: {\"ok\":true}."},
			{"role": "user", "content": "Preflight the connection."},
		},
	}
	if rf := buildProbeResponseFormat(responseMode); rf != nil {
		request["response_format"] = rf
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

// buildProbeResponseFormat constructs the response_format field for the
// preflight probe using the same llm types sent during actual revision.
func buildProbeResponseFormat(mode string) *ResponseFormat {
	switch mode {
	case "json_object":
		return &ResponseFormat{Type: "json_object"}
	case "json_schema":
		return &ResponseFormat{
			Type: "json_schema",
			JSONSchema: &JSONSchema{
				Name:   "preflight",
				Schema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
				Strict: true,
			},
		}
	default:
		return nil
	}
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
	body, err := io.ReadAll(io.LimitReader(response.Body, probeMaxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > probeMaxResponse {
		return nil, errors.New("preflight response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		message := strings.TrimSpace(string(body))
		if apiKey != "" {
			message = strings.ReplaceAll(message, apiKey, "[REDACTED]")
		}
		message = SafeDisplay(message)
		return nil, fmt.Errorf("HTTP %d: %s", response.StatusCode, message)
	}
	return body, nil
}

// ProbeMode probes one response mode against the endpoint.
func ProbeMode(ctx context.Context, client *http.Client, baseURL, model, apiKey, responseMode string) error {
	return probeMode(ctx, client, baseURL, model, apiKey, responseMode)
}
