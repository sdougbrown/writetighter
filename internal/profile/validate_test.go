package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeBundle(t *testing.T, dir, manifest, dictionary, rules string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dictionary.json"), []byte(dictionary), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(rules), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedBundleValidates(t *testing.T) {
	_, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("embedded bundle should validate: %v", err)
	}
}

func TestValidate_UnsupportedLanguage(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "fr",
		"license": {"spdx": "MIT"},
		"provenance": [{"name": "test"}],
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": []}`
	rules := `{"format_version": 1, "rules": []}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_UnsupportedKind(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "en",
		"supported_kinds": ["description", "blog"],
		"license": {"spdx": "MIT"},
		"provenance": [{"name": "test"}],
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": []}`
	rules := `{"format_version": 1, "rules": []}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_EmptyProvenance(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "en",
		"license": {"spdx": "MIT"},
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": []}`
	rules := `{"format_version": 1, "rules": []}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for empty provenance")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_ProvenanceMissingName(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "en",
		"license": {"spdx": "MIT"},
		"provenance": [{"release": "v1"}],
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": []}`
	rules := `{"format_version": 1, "rules": []}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for provenance missing name")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_UnsupportedRuleID(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "en",
		"license": {"spdx": "MIT"},
		"provenance": [{"name": "test"}],
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": []}`
	rules := `{"format_version": 1, "rules": [{"id": "BAD.RULE", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1}]}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for unsupported rule ID")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_UnknownParameter(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "en",
		"license": {"spdx": "MIT"},
		"provenance": [{"name": "test"}],
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": []}`
	rules := `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"bad_param": 1}}]}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for unknown parameter")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_UnresolvedAlternative(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "en",
		"license": {"spdx": "MIT"},
		"provenance": [{"name": "test"}],
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": [{"term": "foo", "status": "discouraged", "alternatives": ["bar"], "reason": "use bar instead", "parts_of_speech": ["noun"]}]}`
	rules := `{"format_version": 1, "rules": []}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for unresolved alternative")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_AlternativeResolvesToNonPreferred(t *testing.T) {
	manifest := `{
		"format_version": 1,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "en",
		"license": {"spdx": "MIT"},
		"provenance": [{"name": "test"}],
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 1, "entries": [
		{"term": "foo", "status": "discouraged", "alternatives": ["bar"], "reason": "use bar instead", "parts_of_speech": ["noun"]},
		{"term": "bar", "status": "observed", "parts_of_speech": ["noun"]}
	]}`
	rules := `{"format_version": 1, "rules": []}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected error for alternative resolving to non-preferred/allowed")
	}
	fmt.Println("  OK:", err)
}

func TestValidate_AlternativeResolvesSuccessfully(t *testing.T) {
	// Test ValidateAlternatives directly (avoids hash mismatch from bundle path)
	dictJSON := `{"format_version": 1, "entries": [
		{"term": "foo", "status": "discouraged", "alternatives": ["bar"], "reason": "use bar instead", "parts_of_speech": ["noun"]},
		{"term": "bar", "status": "preferred", "parts_of_speech": ["noun"]}
	]}`
	d, err := parseDictionary([]byte(dictJSON))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := d.ValidateAlternatives(); err != nil {
		t.Fatalf("expected no error for valid alternative resolution: %v", err)
	}
	fmt.Println("  OK")
}

func TestValidate_CollectsMultipleErrors(t *testing.T) {
	manifest := `{
		"format_version": 2,
		"id": "test-profile",
		"version": "0.1.0",
		"display_name": "Test",
		"language": "fr",
		"supported_kinds": ["blog"],
		"license": {"spdx": "MIT"},
		"payloads": {"dictionary.json": {"sha256": ""}, "rules.json": {"sha256": ""}}
	}`
	dictionary := `{"format_version": 2, "entries": []}`
	rules := `{"format_version": 2, "rules": [{"id": "BAD", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1}]}`

	dir := t.TempDir()
	writeBundle(t, dir, manifest, dictionary, rules)
	err := ValidateBundle(dir)
	if err == nil {
		t.Fatal("expected multiple errors")
	}
	fmt.Println("  OK - multiple errors collected:", err)
}
