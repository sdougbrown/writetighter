package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// releaseDir resolves the repo's embedded release source directory so tests
// can drive the real format-3 bundle on disk.
func releaseDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("data", "profiles", "software-docs-en", "0.7.0"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatalf("release fixture missing: %v", err)
	}
	return dir
}

func TestFormatThreeBundleLoadsInPlace(t *testing.T) {
	res, err := LoadBundle(releaseDir(t))
	if err != nil {
		t.Fatalf("format 3 bundle failed to load: %v", err)
	}
	if res.Version != Version("0.7.0") {
		t.Fatalf("unexpected version %q", res.Version)
	}
	// The lexicon's word classes must be visible through the merged view.
	if !res.Dict.HasWordClass("the", "determiner") {
		t.Fatal("expected lexicon word classes in merged dictionary view")
	}
	if res.Dict.Lookup("a lot of") == nil {
		t.Fatal("expected dictionary entries in merged view")
	}
}

func TestFormatThreeBundleTamperedLexiconRejected(t *testing.T) {
	src := releaseDir(t)
	dir := t.TempDir()
	for _, name := range []string{"manifest.json", "rules.json", "lexicon.json"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lexPath := filepath.Join(dir, "lexicon.json")
	b, _ := os.ReadFile(lexPath)
	if err := os.WriteFile(lexPath, append(b, []byte("\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBundle(dir)
	if err == nil || !strings.Contains(err.Error(), "lexicon.json hash mismatch") {
		t.Fatalf("expected lexicon hash mismatch, got: %v", err)
	}
}

func TestFormatThreeBundleUnknownPayloadRejected(t *testing.T) {
	src := releaseDir(t)
	dir := t.TempDir()
	for _, name := range []string{"manifest.json", "rules.json", "lexicon.json"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	b, _ := os.ReadFile(manifestPath)
	tampered := strings.Replace(string(b),
		`"lexicon.json": {`, `"extra.json": {"sha256": "z"}, "lexicon.json": {`, 1)
	if tampered == string(b) {
		t.Fatal("failed to tamper manifest")
	}
	if err := os.WriteFile(manifestPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBundle(dir)
	if err == nil || !strings.Contains(err.Error(), "unknown payload") {
		t.Fatalf("expected unknown payload rejection, got: %v", err)
	}
}

func TestFormatThreeBundleUnresolvableDictionaryRejected(t *testing.T) {
	src := releaseDir(t)
	dir := t.TempDir()
	for _, name := range []string{"rules.json", "lexicon.json"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Manifest with a dictionary hash that matches nothing in the store.
	manifest := `{
  "claims": {"certification": "none", "issue": null, "standard": null},
  "display_name": "Software Documentation English",
  "format_version": 3,
  "id": "software-docs-en",
  "language": "en",
  "license": {"spdx": "LicenseRef-Private-Research"},
  "payloads": {
    "dictionary": {"sha256": "0000000000000000000000000000000000000000000000000000000000000000"},
    "lexicon.json": {"sha256": "x"},
    "rules.json": {"sha256": "y"}
  },
  "provenance": [{"license": "MIT", "name": "x", "release": "1", "source_url": "https://example.com"}],
  "supported_kinds": ["description"],
  "version": "9.9.9"
}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBundle(dir)
	if err == nil || !strings.Contains(err.Error(), "no embedded dictionary matches") {
		t.Fatalf("expected unresolvable dictionary rejection, got: %v", err)
	}
}

func TestFormatThreeInstallMaterializesDictionary(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataRoot)
	// Copy the release and re-version it so install does not hit the
	// embedded-equivalence no-op: the point is the materialization path.
	src := releaseDir(t)
	fixture := filepath.Join(dataRoot, "fixture")
	if err := os.MkdirAll(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest.json", "rules.json", "lexicon.json"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixture, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(fixture, "manifest.json")
	b, _ := os.ReadFile(manifestPath)
	if err := os.WriteFile(manifestPath, []byte(strings.Replace(string(b), `"version": "0.7.0"`, `"version": "9.9.9"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallBundle(fixture); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	installed := filepath.Join(profileRoot(), "software-docs-en", "9.9.9")
	res, err := LoadBundle(installed)
	if err != nil {
		t.Fatalf("installed bundle failed to load: %v", err)
	}
	if res.Version != Version("9.9.9") {
		t.Fatalf("unexpected version %q", res.Version)
	}
	if _, err := os.Stat(filepath.Join(installed, "dictionary.json")); err != nil {
		t.Fatal("expected materialized dictionary.json in installed bundle")
	}
}
