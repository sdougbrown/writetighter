package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ModelInfo describes a model returned by the /v1/models endpoint.
type ModelInfo struct {
	ID               string `json:"id"`
	ContextLength    int    `json:"context_length,omitempty"`
	MaxContextLength int    `json:"max_context_length,omitempty"`
	MaxModelLen      int    `json:"max_model_len,omitempty"`
}

// SuggestedContextWindow returns the suggested context window from model
// metadata, or 0 if no metadata is available. The precedence order is:
// context_length, max_context_length, max_model_len.
func (m ModelInfo) SuggestedContextWindow() int {
	if m.ContextLength > 0 {
		return m.ContextLength
	}
	if m.MaxContextLength > 0 {
		return m.MaxContextLength
	}
	if m.MaxModelLen > 0 {
		return m.MaxModelLen
	}
	return 0
}

type modelsEnvelope struct {
	Data []ModelInfo `json:"data"`
}

const modelsMaxResponse = 1 << 20

// ListModels queries the /v1/models endpoint and returns discovered models
// with metadata.
func ListModels(baseURL, apiKey string, timeout time.Duration) ([]ModelInfo, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelsMaxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > modelsMaxResponse {
		return nil, fmt.Errorf("models response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		message := strings.TrimSpace(string(body))
		if apiKey != "" {
			message = strings.ReplaceAll(message, apiKey, "[REDACTED]")
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, SafeDisplay(message))
	}
	var envelope modelsEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	seen := make(map[string]struct{})
	models := make([]ModelInfo, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		id := strings.TrimSpace(item.ID)
		if !validModelID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		item.ID = id
		models = append(models, item)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// LookupContextWindow queries the /v1/models endpoint and returns the
// suggested context window for the named model, or 0 if the model is not
// found or has no metadata.
func LookupContextWindow(baseURL, apiKey, model string, timeout time.Duration) (int, error) {
	models, err := ListModels(baseURL, apiKey, timeout)
	if err != nil {
		return 0, err
	}
	for _, m := range models {
		if m.ID == model {
			return m.SuggestedContextWindow(), nil
		}
	}
	return 0, nil
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

// SafeDisplay replaces every Unicode control character with a space.
func SafeDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}
