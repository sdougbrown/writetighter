package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/profile"
	"github.com/sdougbrown/writetighter/internal/report"
)

func testDoc() *document.Document {
	doc, _ := document.FromReader(strings.NewReader("deprecated term appears here."), "test.md", "")
	return doc
}

func testProfile() *profile.Resolution {
	return &profile.Resolution{ID: "PROFILE_ID", Version: "1", Rules: &profile.RulesConfig{Rules: []profile.Rule{{ID: "CORE.TERM_DISCOURAGED", Enabled: true}}}, Dict: &profile.Dictionary{Entries: []profile.Entry{{Term: "deprecated term", Status: profile.StatusDiscouraged}}}}
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

func TestAdversarialText(t *testing.T) {
	resp := `{"findings":[{"source_range":{"start":0,"end":5},"rule_ids":["CORE.TERM_DISCOURAGED"],"reason":"claim","replacement":"certified compliance guaranteed","confidence":1}]}`
	_, err := ValidateAdvisorResponse([]byte(resp), "hello world")
	if err == nil {
		t.Fatal("expected claim rejection")
	}
}

func TestRequiredOptionalFailure(t *testing.T) {
	doc := testDoc()
	res := testProfile()
	_, err := Advisor(context.Background(), Config{BaseURL: "http://127.0.0.1:1", Model: "gpt", Timeout: time.Millisecond}, doc, res, []report.Finding{{RuleID: "CORE.TERM_DISCOURAGED", Message: "x", Range: &report.FindingRange{StartByte: 0, EndByte: 5}}})
	if err == nil {
		t.Fatal("expected advisor failure")
	}
}
