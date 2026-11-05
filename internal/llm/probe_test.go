package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"
)

func TestDoSanitizesControlCharactersInNon2xxBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("\x1b[2Jbad response\n"))
	}))
	defer server.Close()

	_, err := doJSON(context.Background(), server.Client(), http.MethodGet, server.URL+"/", "", nil)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "HTTP 418") {
		t.Fatalf("error should contain HTTP status: %s", errStr)
	}
	if !strings.Contains(errStr, "bad response") {
		t.Fatalf("error should contain body message: %s", errStr)
	}
	for i, r := range errStr {
		if unicode.IsControl(r) {
			t.Fatalf("error contains control character at byte index %d: %q", i, string(r))
		}
	}
}
