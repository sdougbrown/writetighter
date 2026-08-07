package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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