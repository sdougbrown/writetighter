package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/sdougbrown/writetighter/internal/app"
	"github.com/sdougbrown/writetighter/internal/profile"

	clicontract "github.com/sdougbrown/cli-contract"
)

func TestVersionNoEmbeddedProfile(t *testing.T) {
	oldLoader := loadEmbedded
	loadEmbedded = func() (*profile.Resolution, error) { return nil, nil }
	defer func() { loadEmbedded = oldLoader }()

	var out bytes.Buffer
	if got := Run([]string{"version"}, &out, io.Discard); got != 0 {
		t.Fatalf("exit %d", got)
	}
	text := out.String()
	if strings.Contains(text, "embedded profile:") {
		t.Fatalf("embedded profile line should be absent, got: %s", text)
	}
	if !strings.Contains(text, "writetighter "+app.Version) {
		t.Fatalf("expected 'writetighter %s' in output, got: %s", app.Version, text)
	}

	// --json with no embedded profile: embedded_profiles must be empty
	out.Reset()
	if got := Run([]string{"version", "--json"}, &out, io.Discard); got != 0 {
		t.Fatalf("exit %d", got)
	}
	var payload struct {
		EmbeddedProfiles []struct {
			ID string `json:"id"`
		} `json:"embedded_profiles"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(payload.EmbeddedProfiles) != 0 {
		t.Fatalf("expected empty embedded_profiles, got %d items", len(payload.EmbeddedProfiles))
	}
}

func TestExitCodeMapsFailureKinds(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"no failure", nil, ExitOK},
		{"fail threshold", app.ErrFailThreshold, ExitFailThreshold},
		{"revise failed", app.ErrReviseFailed, ExitModel},
		{"rewrite failed", app.ErrRewriteFailed, ExitModel},
		{"wrapped revise", fmt.Errorf("%w: boom", app.ErrReviseFailed), ExitModel},
		{"other failure", io.ErrUnexpectedEOF, ExitUsage},
	}
	for _, tc := range cases {
		if got := (cliAdapter{}).ExitCode(clicontract.Invocation{}, nil, tc.err); got != tc.want {
			t.Errorf("%s: ExitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}
