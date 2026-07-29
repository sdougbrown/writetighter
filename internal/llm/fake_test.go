package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

type fakeLLMServer struct {
	server *httptest.Server
	URL    string
}

func newFakeServer(requireKey bool, mode string) *fakeLLMServer {
	f := &fakeLLMServer{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireKey && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch mode {
		case "malformed":
			_, _ = w.Write([]byte(`not-json`))
		case "timeout":
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusGatewayTimeout)
		case "oversized-envelope":
			_, _ = w.Write([]byte(strings.Repeat("x", MaxEnvelopeChars+1)))
		default:
			content := `{"findings":[]}`
			if mode == "escaped-envelope" {
				content = strings.Repeat(`"`, 8000)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-test",
				"model":   "test-model",
				"choices": []Choice{{Message: Message{Role: "assistant", Content: content}}},
				"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 1},
			})
		}
	}))
	f.URL = f.server.URL
	return f
}

func (f *fakeLLMServer) Close() { f.server.Close() }

// newFakeReviseServer returns responses matching the revision schema.
func newFakeReviseServer(requireKey bool, mode string) *fakeLLMServer {
	f := &fakeLLMServer{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireKey && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch mode {
		case "malformed":
			w.Write([]byte(`not-json`))
		case "timeout":
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusGatewayTimeout)
		default:
			// Build revise-compatible content JSON.
			type frev struct {
				Kind         string      `json:"kind"`
				SourceText   string      `json:"source_text"`
				SourceRange  SourceRange `json:"source_range"`
				PrincipleIDs []string    `json:"principle_ids"`
				Reason       string      `json:"reason"`
				Replacement  string      `json:"replacement"`
				Confidence   float64     `json:"confidence"`
			}
			finding := frev{
				Kind:         "rewrite",
				SourceText:   "depre",
				SourceRange:  SourceRange{Start: 0, End: 5},
				PrincipleIDs: []string{"CORE.SHORT_SENTENCE"},
				Reason:       "rewrite suggestion",
				Replacement:  "preferred term",
				Confidence:   0.91,
			}
			if mode == "wrong-range" {
				finding.SourceText = "term"
			}
			respContent, _ := json.Marshal(map[string]any{"findings": []frev{finding}})
			json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-test",
				"model":   "test-model",
				"choices": []Choice{{Message: Message{Role: "assistant", Content: string(respContent)}}},
				"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 1},
			})
		}
	}))
	f.URL = f.server.URL
	return f
}
