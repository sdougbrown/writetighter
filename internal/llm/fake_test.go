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
		default:
			content := `{"findings":[{"source_range":{"start":0,"end":5},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"rewrite suggestion","replacement":"preferred term","confidence":0.91}]}`
			_ = json.NewEncoder(w).Encode(Response{Choices: []Choice{{Message: Message{Role: "assistant", Content: content}}}})
		}
	}))
	f.URL = f.server.URL
	return f
}

func (f *fakeLLMServer) Close() { f.server.Close() }
