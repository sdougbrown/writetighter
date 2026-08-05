package codecomment

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestExtractAssignsStableUTF8SpansAndCoalescesFullLines(t *testing.T) {
	source := []byte("package p\r\n\t// café rationale\r\n\t// applies before parsing\r\nfunc f() {} // trailing\r\n\n// separate\r\n")
	catalog, err := Extract("sample.go", Go, source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(catalog.Comments), 3; got != want {
		t.Fatalf("comments = %d, want %d: %#v", got, want, catalog.Comments)
	}
	first := catalog.Comments[0]
	if first.ID != "c0001" || first.Form != LineComment || first.Span.StartLine != 2 || first.Span.EndLine != 3 {
		t.Fatalf("unexpected first comment: %#v", first)
	}
	if want := "// café rationale\r\n\t// applies before parsing"; first.Text != want {
		t.Fatalf("first text = %q, want %q", first.Text, want)
	}
	if !bytes.Equal(source[first.Span.StartByte:first.Span.EndByte], []byte(first.Text)) {
		t.Fatal("span does not select exact source bytes")
	}
	if catalog.Comments[1].Text != "// trailing" || catalog.Comments[2].Text != "// separate" {
		t.Fatalf("trailing or blank-line separation lost: %#v", catalog.Comments)
	}
	hash := sha256.Sum256(source)
	if catalog.SourceSHA256 != fmt.Sprintf("%x", hash) {
		t.Fatalf("source hash = %q", catalog.SourceSHA256)
	}
}

func TestExtractGoUsesScannerAndRejectsMalformedInput(t *testing.T) {
	catalog, err := Extract("sample.go", Go, []byte("package p\nvar u = \"https://example.test/x\"\n// real\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Comments[0].Text; got != "// real" {
		t.Fatalf("comment = %q", got)
	}
	if _, err := Extract("bad.go", Go, []byte("package p\n/* no close")); err == nil {
		t.Fatal("unterminated Go comment was accepted")
	}
}

func TestExtractTypeScriptSkipsTemplatesAndRegexButScansTemplateExpressions(t *testing.T) {
	source := []byte("const pattern = /https?:\\/\\/example\\.test\\/\\/path/;\nconst text = `// text ${1 /* expression */}`;\n// real\n")
	catalog, err := Extract("sample.ts", TypeScript, source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := commentTexts(catalog), []string{"/* expression */", "// real"}; !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
	for _, source := range [][]byte{
		[]byte("const x = `unterminated"),
		[]byte("const x = /unterminated\n// not partial"),
		[]byte("/* unterminated"),
	} {
		if _, err := Extract("bad.ts", TypeScript, source); err == nil {
			t.Fatalf("malformed TypeScript accepted: %q", source)
		}
	}
}

func TestExtractTypeScriptSkipsRegexLiteralsAfterExpressionKeywords(t *testing.T) {
	source := []byte("function f() { return /[//]/; }\nfunction g() { throw /[//]/; }\nswitch (value) { case /[//]/.source: break; }\nconst kind = typeof /[//]/;\nfunction h(ready: boolean, value: string) {\n  if (ready) /[//]/.test(value);\n  while (false) /[//]/.test(value);\n  for (;;) /[//]/.test(value);\n}\n// real\n")
	catalog, err := Extract("sample.ts", TypeScript, source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := commentTexts(catalog), []string{"// real"}; !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
}

func TestExtractSkipsCommentDelimitersInsideSupportedLiteralForms(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		language Language
		source   []byte
		want     []string
	}{
		{
			name:     "Go raw string",
			filename: "sample.go",
			language: Go,
			source:   []byte("package p\nvar raw = `// not a comment /* not a comment */`\n// real\n"),
			want:     []string{"// real"},
		},
		{
			name:     "Rust C raw and quoted strings",
			filename: "sample.rs",
			language: Rust,
			source:   []byte("let raw = cr##\"// not a comment /* not a comment */\"##;\nlet text = \"// not a comment\";\n// real\n"),
			want:     []string{"// real"},
		},
		{
			name:     "Python raw string",
			filename: "sample.py",
			language: Python,
			source: []byte(`value = r"# not a comment"
# real
`),
			want: []string{"# real"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog, err := Extract(test.filename, test.language, test.source)
			if err != nil {
				t.Fatal(err)
			}
			if got := commentTexts(catalog); !equalStrings(got, test.want) {
				t.Fatalf("comments = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestExtractRustHandlesNestedCommentsAndRawByteAndCharacterStrings(t *testing.T) {
	source := []byte("let raw = br###\"/* not */ // not\"###;\nlet plain = r#\"/* not */ // not, trailing #\"#;\nlet ch = 'x'; let byte = b'\\\\'; let lifetime: &'a str;\n/* outer /* inner */ outer */\n// real\n")
	catalog, err := Extract("sample.rs", Rust, source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := commentTexts(catalog), []string{"/* outer /* inner */ outer */", "// real"}; !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
	if _, err := Extract("bad.rs", Rust, []byte("/* outer /* inner */")); err == nil {
		t.Fatal("unterminated nested Rust comment was accepted")
	}
	if _, err := Extract("bad.rs", Rust, []byte("let ch = '\\x\n// not partial")); err == nil {
		t.Fatal("unterminated Rust character literal was accepted")
	}
	if _, err := Extract("bad.rs", Rust, []byte("let raw = r##\"unterminated")); err == nil {
		t.Fatal("unterminated Rust raw string was accepted")
	}
}

func TestReplacementIsSafePreservesCommentBoundariesAndForm(t *testing.T) {
	source := []byte("package p\n// full line\nfunc f() {} // inline\n")
	catalog, err := Extract("sample.go", Go, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Comments) != 2 {
		t.Fatalf("comments = %#v", catalog.Comments)
	}
	if !ReplacementIsSafe(Go, source, catalog.Comments[0], "// clearer full line") {
		t.Fatal("safe line-comment replacement was rejected")
	}
	if ReplacementIsSafe(Go, source, catalog.Comments[0], "/* changed form */") {
		t.Fatal("replacement with a different comment form was accepted")
	}
	if !ReplacementIsSafe(Go, source, catalog.Comments[1], "// clearer inline comment") {
		t.Fatal("safe inline-comment replacement was rejected")
	}
	if ReplacementIsSafe(Go, source, catalog.Comments[1], "// comment\nexecutable()") {
		t.Fatal("replacement containing executable code was accepted")
	}
}

func TestExtractPythonTreatsDocstringsAndFormattedStringsAsStrings(t *testing.T) {
	source := []byte("\"\"\"module # documentation\"\"\"\nvalue = fr\"# not a comment {value}\"\n# actual café\n")
	catalog, err := Extract("sample.py", Python, source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := commentTexts(catalog), []string{"# actual café"}; !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
	if _, err := Extract("bad.py", Python, []byte("x = '''unterminated")); err == nil {
		t.Fatal("unterminated Python string was accepted")
	}
}

func TestLanguageDetection(t *testing.T) {
	for _, test := range []struct {
		name string
		want Language
		ok   bool
	}{
		{"file.go", Go, true}, {"file.tsx", TypeScript, true}, {"file.js", TypeScript, true}, {"file.rs", Rust, true}, {"file.py", Python, true}, {"file.txt", "", false},
	} {
		got, ok := DetectLanguage(test.name)
		if got != test.want || ok != test.ok {
			t.Errorf("DetectLanguage(%q) = %q, %t; want %q, %t", test.name, got, ok, test.want, test.ok)
		}
	}
}

func commentTexts(c Catalog) []string {
	out := make([]string, len(c.Comments))
	for i, comment := range c.Comments {
		out[i] = comment.Text
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
