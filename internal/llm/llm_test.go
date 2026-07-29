package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sdougbrown/writetighter/internal/config"
	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
)

func testDoc() *document.Document {
	doc, _ := document.FromReader(strings.NewReader("deprecated term appears here."), "test.md", "")
	return doc
}

func testProfile() *profile.Resolution {
	return &profile.Resolution{ID: "PROFILE_ID", Version: "1", Rules: &profile.RulesConfig{Rules: []profile.Rule{{ID: "CORE.TERM_DISCOURAGED", Version: 1, Enabled: true}}}, Dict: &profile.Dictionary{Entries: []profile.Entry{{Term: "deprecated term", Status: profile.StatusDiscouraged}}}}
}

func TestClientPreservesAPIRootPath(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://example.test/openai/v1", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := client.Endpoint(), "https://example.test/openai/v1/chat/completions"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

func TestClientRejectsCredentialsInBaseURL(t *testing.T) {
	_, err := NewClient(Config{BaseURL: "https://secret@example.test/v1", Model: "test"})
	if err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
		t.Fatalf("expected embedded credential rejection, got %v", err)
	}
}

func TestCombinedInputLimit(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "http://127.0.0.1:1", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), Request{Messages: []Message{
		{Role: "system", Content: strings.Repeat("a", MaxInputChars/2+1)},
		{Role: "user", Content: strings.Repeat("b", MaxInputChars/2+1)},
	}})
	if err == nil || !strings.Contains(err.Error(), "input too large") {
		t.Fatalf("combined input error = %v", err)
	}
}

func TestTruncatedExcerptExclusiveEndBeforeGap(t *testing.T) {
	excerpt := newExcerpt("abc\nxyz", []int{0, 1, 2, -1, 10, 11, 12})
	truncated := truncateExcerpt(excerpt, 3)
	if got := truncated.OrigOffset(len(truncated.Text)); got != 3 {
		t.Fatalf("exclusive original end = %d, want 3", got)
	}
}

func TestNoAuthorizationHeader(t *testing.T) {
	srv := newFakeServer(false, "ok")
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil || len(resp.Choices) == 0 {
		t.Fatalf("expected response, got %v %v", resp, err)
	}
}

func TestStoredKeyIsRedactedFromHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rejected "+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Model: "gpt", APIKey: "pat-sensitive", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil || strings.Contains(err.Error(), "pat-sensitive") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("expected redacted HTTP error, got %v", err)
	}
}

func TestStoredKeyMode(t *testing.T) {
	srv := newFakeServer(true, "ok")
	defer srv.Close()
	client, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", APIKey: "local-pat", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsTwoKeySources(t *testing.T) {
	_, err := NewClient(Config{BaseURL: "http://localhost:4000/v1", Model: "gpt", APIKey: "one", APIKeyEnv: "OTHER"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected conflicting key sources to fail, got %v", err)
	}
}

func TestKeyedMode(t *testing.T) {
	srv := newFakeServer(true, "ok")
	defer srv.Close()
	_ = os.Setenv("PROSEVET_API_KEY", "secret")
	defer os.Unsetenv("PROSEVET_API_KEY")
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", APIKeyEnv: "PROSEVET_API_KEY", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestResponseFallback(t *testing.T) {
	srv := newFakeServer(false, "malformed")
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestClientAllowsEscapedEnvelopeLargerThanValidatedContentLimit(t *testing.T) {
	srv := newFakeServer(false, "escaped-envelope")
	defer srv.Close()
	client, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil || len(response.Choices) != 1 || len(response.Choices[0].Message.Content) != 8000 {
		t.Fatalf("expected bounded escaped envelope to decode, got %#v, err=%v", response, err)
	}
}

func TestClientRejectsOversizedEnvelope(t *testing.T) {
	srv := newFakeServer(false, "oversized-envelope")
	defer srv.Close()
	client, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "envelope too large") {
		t.Fatalf("expected oversized envelope error, got %v", err)
	}
}

func TestTimeoutHandling(t *testing.T) {
	srv := newFakeServer(false, "timeout")
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, Model: "gpt", Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestByteOffsetToLineColumn(t *testing.T) {
	content := "Hello\nWorld\n"
	tests := []struct {
		offset   int
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},
		{3, 1, 4},  // "Hel" -> col 4
		{6, 2, 1},  // after first \n
		{11, 2, 6}, // "World" -> col 6
		{12, 3, 1}, // after second \n
	}
	for _, tt := range tests {
		line, col := byteOffsetToLineColumn(content, tt.offset)
		if line != tt.wantLine || col != tt.wantCol {
			t.Errorf("byteOffsetToLineColumn(%q, %d) = (%d, %d), want (%d, %d)",
				content, tt.offset, line, col, tt.wantLine, tt.wantCol)
		}
	}
}

func TestPreservesProtectedContent(t *testing.T) {
	content := "Configure WidgetAPI v1.2.3 with `widgetctl --mode safe` at /etc/widget/config.yaml for 12 quiet PRs; notify ops@example.com about host 192.0.2.4 and request 123e4567-e89b-12d3-a456-426614174000."
	doc, err := document.FromReader(strings.NewReader(content), "test.md", "description")
	if err != nil {
		t.Fatal(err)
	}
	terms := []config.TermEntry{{Term: "quiet PRs"}}
	preserved := "For 12 quiet PRs, configure WidgetAPI v1.2.3 with `widgetctl --mode safe` at /etc/widget/config.yaml; notify ops@example.com about host 192.0.2.4 and request 123e4567-e89b-12d3-a456-426614174000."
	if !preservesProtectedContent(doc, 0, len(content), preserved, terms) {
		t.Fatal("expected exact technical content to be accepted")
	}
	missingCommand := "For 12 quiet PRs, configure WidgetAPI v1.2.3 at /etc/widget/config.yaml; notify ops@example.com about host 192.0.2.4 and request 123e4567-e89b-12d3-a456-426614174000."
	if preservesProtectedContent(doc, 0, len(content), missingCommand, terms) {
		t.Fatal("expected rewrite that drops inline code to be rejected")
	}
	changedVersion := "For 12 quiet PRs, configure WidgetAPI v1.2.4 with `widgetctl --mode safe` at /etc/widget/config.yaml; notify ops@example.com about host 192.0.2.4 and request 123e4567-e89b-12d3-a456-426614174000."
	if preservesProtectedContent(doc, 0, len(content), changedVersion, terms) {
		t.Fatal("expected rewrite that changes a version to be rejected")
	}
	changedTerm := "For 12 silent PRs, configure WidgetAPI v1.2.3 with `widgetctl --mode safe` at /etc/widget/config.yaml; notify ops@example.com about host 192.0.2.4 and request 123e4567-e89b-12d3-a456-426614174000."
	if preservesProtectedContent(doc, 0, len(content), changedTerm, terms) {
		t.Fatal("expected rewrite that changes a defined project term to be rejected")
	}
}

func TestByteOffsetToLineColumnUTF8(t *testing.T) {
	// "\u00fcber" is 5 bytes, 4 code points
	content := "\u00fcber"
	line, col := byteOffsetToLineColumn(content, 0)
	if line != 1 || col != 1 {
		t.Fatalf("offset 0: got (%d,%d), want (1,1)", line, col)
	}
	line, col = byteOffsetToLineColumn(content, 2) // after 0xC3 0xBC
	if line != 1 || col != 2 {
		t.Fatalf("offset 2 (after '\u00fc'): got (%d,%d), want (1,2)", line, col)
	}
	line, col = byteOffsetToLineColumn(content, 5) // end
	if line != 1 || col != 5 {
		t.Fatalf("offset 5 (end): got (%d,%d), want (1,5)", line, col)
	}
}
