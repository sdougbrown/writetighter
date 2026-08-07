package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLookupContextWindowFindsModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "gemma4", "context_length": 8192},
			{"id": "qwen", "max_model_len": 4096},
		}})
	}))
	defer server.Close()

	cw, err := LookupContextWindow(server.URL, "", "gemma4", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if cw != 8192 {
		t.Fatalf("context window = %d, want 8192", cw)
	}

	cw, err = LookupContextWindow(server.URL, "", "qwen", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if cw != 4096 {
		t.Fatalf("context window = %d, want 4096 (from max_model_len)", cw)
	}
}

func TestLookupContextWindowModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "gemma4", "context_length": 8192},
		}})
	}))
	defer server.Close()

	cw, err := LookupContextWindow(server.URL, "", "nonexistent", 10*time.Second)
	if err != nil {
		t.Fatalf("model not found should return 0, nil, not error: %v", err)
	}
	if cw != 0 {
		t.Fatalf("context window = %d, want 0 for missing model", cw)
	}
}

func TestLookupContextWindowNoMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "gemma4"},
		}})
	}))
	defer server.Close()

	cw, err := LookupContextWindow(server.URL, "", "gemma4", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if cw != 0 {
		t.Fatalf("context window = %d, want 0 for model without metadata", cw)
	}
}

func TestLookupContextWindowEndpointError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := LookupContextWindow(server.URL, "", "gemma4", 10*time.Second)
	if err == nil {
		t.Fatal("expected error for endpoint failure")
	}
}

func TestListModelsRejectsLargeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The response does not need to be valid JSON: the size check runs first.
		_, _ = w.Write([]byte(strings.Repeat("x", modelsMaxResponse+1)))
	}))
	defer server.Close()

	_, err := ListModels(server.URL, "", 10*time.Second)
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !strings.Contains(err.Error(), "models response is too large") {
		t.Fatalf("expected 'models response is too large', got: %v", err)
	}
}

func TestListModelsDropsControlCharacterIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "gemma4", "context_length": 4096},
			{"id": "bad\x1b[2J"},
		}})
	}))
	defer server.Close()

	models, err := ListModels(server.URL, "", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gemma4" {
		t.Fatalf("models = %#v", models)
	}
	if models[0].SuggestedContextWindow() != 4096 {
		t.Fatalf("SuggestedContextWindow = %d, want 4096", models[0].SuggestedContextWindow())
	}
}

func TestListModelsDropsEmptyIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "gemma4", "context_length": 4096},
			{"id": ""},
		}})
	}))
	defer server.Close()

	models, err := ListModels(server.URL, "", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gemma4" {
		t.Fatalf("models = %#v", models)
	}
}

func TestListModelsDropsLongIDs(t *testing.T) {
	longID := strings.Repeat("a", 257)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "gemma4", "context_length": 4096},
			{"id": longID},
		}})
	}))
	defer server.Close()

	models, err := ListModels(server.URL, "", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gemma4" {
		t.Fatalf("models = %#v", models)
	}
}

func TestSuggestedContextWindowPrecedence(t *testing.T) {
	m := ModelInfo{
		ID:               "test-model",
		ContextLength:    16384,
		MaxContextLength: 32768,
		MaxModelLen:      65536,
	}

	if got := m.SuggestedContextWindow(); got != 16384 {
		t.Fatalf("SuggestedContextWindow = %d, want 16384 (from ContextLength)", got)
	}
}

func TestValidModelIDRejectsInvalidUTF8(t *testing.T) {
	// Invalid UTF-8 bytes are normalized to U+FFFD by json.Unmarshal,
	// so this path cannot be reached through ListModels. Verify directly.
	if validModelID(string([]byte{0xff, 0xfe})) {
		t.Fatal("expected false for invalid UTF-8")
	}
}

func TestSafeDisplayReplacesControlCharsWithSpaces(t *testing.T) {
	// Input contains ESC sequence (\x1b[2J), newline (\n), tab (\t),
	// carriage return (\r), and printable characters.
	input := "hello\x1b[2J\nworld\tfoo\rbar"
	got := SafeDisplay(input)
	want := "hello [2J world foo bar"
	if got != want {
		t.Fatalf("SafeDisplay(%q) = %q, want %q", input, got, want)
	}
}
