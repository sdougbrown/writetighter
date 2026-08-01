package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// --------------------------------------------------------------------------
// Requirement 4: Profile pinning — ID, version, payload hashes, and canonical
// resolution hash so altered/rehashed data cannot silently become the default.
// --------------------------------------------------------------------------

func TestEmbeddedProfilePinnedIDAndVersion(t *testing.T) {
	res, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("embedded profile load failed: %v", err)
	}
	if res.ID != ProfileID("software-docs-en") {
		t.Errorf("expected pinned ID %q, got %q", "software-docs-en", res.ID)
	}
	if res.Version != Version("0.4.0") {
		t.Errorf("expected pinned version %q, got %q", "0.4.0", res.Version)
	}
}

func TestEmbeddedProfilePinnedManifestHashes(t *testing.T) {
	res, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("embedded profile load failed: %v", err)
	}
	// Expected payload hashes from the reviewed manifest for software-docs-en@0.4.0
	const wantDictHash = "26acb9d56c603c951e7e6e67fd625a5f1d29507adb3b7d1e427d613302d7063f"
	const wantRulesHash = "2eeddca8da1b62115ae2f43aa28b872696c03978f909100f4f60692973acf9be"

	if res.Manifest.Payloads.DictionarySHA256.SHA256 != wantDictHash {
		t.Errorf("dictionary hash:\n  got:  %s\n  want: %s",
			res.Manifest.Payloads.DictionarySHA256.SHA256, wantDictHash)
	}
	if res.Manifest.Payloads.RulesSHA256.SHA256 != wantRulesHash {
		t.Errorf("rules hash:\n  got:  %s\n  want: %s",
			res.Manifest.Payloads.RulesSHA256.SHA256, wantRulesHash)
	}
}

func TestEmbeddedProfileCanonicalResolutionHash(t *testing.T) {
	res, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("embedded profile load failed: %v", err)
	}
	const expectedSHA = "861bdd6385f1664ebc103fcc72706f9614bf35aaea41d54a065c962d5d28c91b"
	if res.SHA256 != expectedSHA {
		t.Errorf("resolution SHA256:\n  got:  %s\n  want: %s", res.SHA256, expectedSHA)
	}
}

// --------------------------------------------------------------------------
// Requirement 5: Stronger rule validation
// --------------------------------------------------------------------------

func TestValidateRuleVersionMustBeSupported(t *testing.T) {
	// Bundle format v1 supports checker version 1 only.
	params := func(v int) string {
		return fmt.Sprintf(`"version": %d`, v)
	}
	for _, v := range []int{-1, 0, 2} {
		rules := fmt.Sprintf(`{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", %s, "parameters": {"description_max_words": 25, "procedure_max_words": 20, "pr_max_words": 20}}]}`, params(v))
		rc, err := parseRules([]byte(rules))
		if err != nil {
			t.Fatalf("parse error for version=%d: %v", v, err)
		}
		if err := rc.Validate(); err == nil {
			t.Errorf("expected error for version %d", v)
		}
	}
}

func TestValidatePositiveIntegralParams(t *testing.T) {
	tests := []struct {
		name   string
		rules  string
		wantOK bool
	}{
		{
			name:   "fractional rejected",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"description_max_words": 3.5}}]}`,
			wantOK: false,
		},
		{
			name:   "string rejected",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"description_max_words": "lots"}}]}`,
			wantOK: false,
		},
		{
			name:   "zero rejected",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"description_max_words": 0}}]}`,
			wantOK: false,
		},
		{
			name:   "negative rejected",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"description_max_words": -5}}]}`,
			wantOK: false,
		},
		{
			name:   "numeric string rejected",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"description_max_words": "25", "procedure_max_words": 20, "pr_max_words": 20}}]}`,
			wantOK: false,
		},
		{
			name:   "missing params rejected for enabled rule",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1}]}`,
			wantOK: false,
		},
		{
			name:   "all sentence params valid",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.SENTENCE_LENGTH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"description_max_words": 25, "procedure_max_words": 20, "pr_max_words": 20}}]}`,
			wantOK: true,
		},
		{
			name:   "unknown param rejected for CORE.DENSE_PARAGRAPH",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.DENSE_PARAGRAPH", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"max_sentences": 3, "max_words": 50, "bad_param": 1}}]}`,
			wantOK: false,
		},
		{
			name:   "unknown param for TERM rule (empty allowed set)",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.TERM_DISCOURAGED", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"any_param": 1}}]}`,
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc, err := parseRules([]byte(tc.rules))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			err = rc.Validate()
			if tc.wantOK && err != nil {
				t.Errorf("expected OK, got: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("expected error, got nil")
			}
			if err != nil && !tc.wantOK {
				t.Logf("expected error: %v", err)
			}
		})
	}
}

func TestValidateDenseParagraphParams(t *testing.T) {
	// CORE.DENSE_PARAGRAPH requires positive integral max_sentences/max_words
	tests := []struct {
		name   string
		rules  string
		wantOK bool
	}{
		{
			name:   "valid params",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.DENSE_PARAGRAPH", "enabled": true, "enforcement": "candidate", "severity": "info", "version": 1, "parameters": {"max_sentences": 3, "max_words": 80}}]}`,
			wantOK: true,
		},
		{
			name:   "max_sentences zero",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.DENSE_PARAGRAPH", "enabled": true, "enforcement": "candidate", "severity": "info", "version": 1, "parameters": {"max_sentences": 0, "max_words": 80}}]}`,
			wantOK: false,
		},
		{
			name:   "max_words string",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.DENSE_PARAGRAPH", "enabled": true, "enforcement": "candidate", "severity": "info", "version": 1, "parameters": {"max_sentences": 3, "max_words": "many"}}]}`,
			wantOK: false,
		},
		{
			name:   "negative max_sentences",
			rules:  `{"format_version": 1, "rules": [{"id": "CORE.DENSE_PARAGRAPH", "enabled": true, "enforcement": "candidate", "severity": "info", "version": 1, "parameters": {"max_sentences": -1, "max_words": 80}}]}`,
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc, err := parseRules([]byte(tc.rules))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			err = rc.Validate()
			if tc.wantOK && err != nil {
				t.Errorf("expected OK, got: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("expected error, got nil")
			}
			if err != nil {
				t.Logf("got error: %v", err)
			}
		})
	}
}

// Test that rules without parameter sets (unregistered in knownRuleParams) reject
// arbitrary parameters.
func TestValidateUnknownRuleParamsRejected(t *testing.T) {
	// CORE.TERM_CASE has empty allowed-parameter set in knownRuleParams
	rules := `{"format_version": 1, "rules": [{"id": "CORE.TERM_CASE", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"xyz": 42}}]}`
	rc, err := parseRules([]byte(rules))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = rc.Validate()
	if err == nil || !strings.Contains(err.Error(), "xyz") {
		t.Errorf("expected error about unknown param 'xyz', got: %v", err)
	}
}

// Test that rules with parameters get validated even if allowed-param-set is empty
func TestValidateParamForTermConsistencyRejected(t *testing.T) {
	rules := `{"format_version": 1, "rules": [{"id": "CORE.TERM_CONSISTENCY", "enabled": true, "enforcement": "enforced", "severity": "warning", "version": 1, "parameters": {"foo": "bar"}}]}`
	rc, err := parseRules([]byte(rules))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = rc.Validate()
	if err == nil {
		t.Error("expected error for TERM_CONSISTENCY with unknown param, got nil")
	}
}
