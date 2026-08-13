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

// TestExtractTSXSkipsJSXMarkupAndFindsJSXContainerComments mirrors the markup
// patterns found in the Umpire docs components (fragments, nested sections,
// self-closing and closing tags, slash-rich text, template children, maps, and
// block comments inside expression containers). Slash-like JSX text must never
// be mistaken for comments, and real container comments must be found.
func TestExtractTSXSkipsJSXMarkupAndFindsJSXContainerComments(t *testing.T) {
	source := []byte("import { useState } from 'react'\n" +
		"\n" +
		"// Component facade: the whole integration surface.\n" +
		"export default function SignupDemo() {\n" +
		"  const [plan, setPlan] = useState<'personal' | 'business'>('personal')\n" +
		"  const planOptions = [\n" +
		"    { value: 'personal', label: 'Personal' },\n" +
		"    { value: 'business', label: 'Business' },\n" +
		"  ]\n" +
		"\n" +
		"  return (\n" +
		"    <>\n" +
		"      <section className=\"c-demo__panel\" data-kind=\"panel/plain\">\n" +
		"        <div className=\"c-demo__title\">\n" +
		"          <span>Plan</span>\n" +
		"          <code>{planLabel(plan)}</code>\n" +
		"        </div>\n" +
		"\n" +
		"        <p className=\"c-demo__note\">\n" +
		"          Rates live at https://example.test/guide/a/b; the winter sale runs 12/25.\n" +
		"          Deps: @preact/signals, no useEffect. 2 > 1 and 1 < 2 stay true.\n" +
		"        </p>\n" +
		"\n" +
		"        <div className=\"c-demo__options\">\n" +
		"          {planOptions.map((option) => (\n" +
		"            <button\n" +
		"              key={option.value}\n" +
		"              type=\"button\"\n" +
		"              aria-label={option.label}\n" +
		"              onClick={() => setPlan(option.value as 'personal' | 'business')}\n" +
		"            >\n" +
		"              {option.label}\n" +
		"            </button>\n" +
		"          ))}\n" +
		"        </div>\n" +
		"\n" +
		"        <input type=\"text\" placeholder=\"First/Last name\" />\n" +
		"        <br />\n" +
		"\n" +
		"        {/* Focus the plan menu on load. */}\n" +
		"        {plan === 'business' && <div className=\"is-active\">Business rate</div>}\n" +
		"      </section>\n" +
		"    </>\n" +
		"  )\n" +
		"}\n")
	catalog, err := Extract("sample.tsx", TypeScript, source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"// Component facade: the whole integration surface.",
		"/* Focus the plan menu on load. */",
	}
	if got := commentTexts(catalog); !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
}

func TestExtractTSXFindsCommentsInJSXAttributeAndChildContainers(t *testing.T) {
	source := []byte(`export default function Demo() {
  const header = (
    <Panel
      title="Summary"
      renderContent={() => (
        /* Header copy: keep it short. */
        <h1>Total</h1>
      )}
    />
  )
  return (
    <div>
      {header}
      {/* Footer copy: handled elsewhere. */}
      <footer>Done.</footer>
    </div>
  )
}
`)
	catalog, err := Extract("sample.tsx", TypeScript, source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/* Header copy: keep it short. */",
		"/* Footer copy: handled elsewhere. */",
	}
	if got := commentTexts(catalog); !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
}

// TestExtractTSXBlocksAndCommentsAroundJSX covers comparison operators,
// generic type parameters, control flow, and generics declared with angle
// brackets, none of which are JSX, plus line comments around them.
func TestExtractTSXBlocksAndCommentsAroundJSX(t *testing.T) {
	source := []byte(`type Pair<T> = { first: T; second: T }
const identity = <T,>(value: T): T => value
function byKind<T>(items: T[]): T[] { return items }

if (pair.first < pair.second) {
  // ordering comment
}
const value = useMemo<number>(() => 0, [])
// trailing comment
`)
	catalog, err := Extract("sample.tsx", TypeScript, source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"// ordering comment", "// trailing comment"}
	if got := commentTexts(catalog); !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
}

func TestExtractTSXFailsClosedOnMalformedInput(t *testing.T) {
	for _, source := range [][]byte{
		[]byte("const x = {/* unterminated"),
		[]byte("const x = <div title=\"unterminated />"),
		[]byte("const x = /unterminated\n"),
		[]byte("return (<> <div>{/* unterminated </div>"),
		[]byte("const x = <div></ invalid>"),
	} {
		if _, err := Extract("bad.tsx", TypeScript, source); err == nil {
			t.Fatalf("malformed TSX accepted: %q", source)
		}
	}
}

// TestExtractTSXScansJSXInsideTemplateInterpolationsAndBlockBodyMaps covers the
// FreightQuote pattern of expression containers with statement bodies, JSX
// inside template interpolations, and slash-rich JSX text: slashes stay text
// and the real comment is found.
func TestExtractTSXScansJSXInsideTemplateInterpolationsAndBlockBodyMaps(t *testing.T) {
	source := []byte("const badge = `status: ${<strong>ready/active</strong>}`\n" +
		"const ratio = `~ 1/2 done`\n" +
		"{fieldGroups.map((group) => {\n" +
		"  const visible = group.fields.filter((field) => !hidden)\n" +
		"  return (\n" +
		"    <div className=\"group\">\n" +
		"      <h2>{group.title}</h2>\n" +
		"      {visible.length > 0 && <p>shows 1/2 of items</p>}\n" +
		"    </div>\n" +
		"  )\n" +
		"})}\n" +
		"// tier comment\n")
	catalog, err := Extract("sample.tsx", TypeScript, source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"// tier comment"}
	if got := commentTexts(catalog); !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
}

func TestJSXEnabled(t *testing.T) {
	for _, tc := range []struct {
		filename string
		want     bool
	}{
		{"sample.tsx", true},
		{"sample.jsx", true},
		{"sample.ts", false},
	} {
		got := jsxEnabled(tc.filename)
		if got != tc.want {
			t.Errorf("jsxEnabled(%q) = %t; want %t", tc.filename, got, tc.want)
		}
	}
}

func TestExtractPlainTypeScriptDoesNotInterpretJSX(t *testing.T) {
	// Plain .ts files keep the exact pre-JSX lexer behavior: '<' is never an
	// element opener, so JSX-shaped source fails closed just as before.
	if _, err := Extract("sample.ts", TypeScript, []byte("const a = <b>c</b>;")); err == nil {
		t.Fatal("plain TypeScript accepted JSX-shaped source")
	}
	// .tsx is required for JSX interpretation.
	catalog, err := Extract("sample.tsx", TypeScript, []byte("const a = <form.Field>{/* real */}c</form.Field>\n// real\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := commentTexts(catalog); !equalStrings(got, []string{"/* real */", "// real"}) {
		t.Fatalf("comments = %#v", got)
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
		{"file.sh", Shell, true}, {"file.bash", Shell, true}, {"file.zsh", Shell, true}, {"file.ksh", Shell, true}, {"file.conf", "", false},
	} {
		got, ok := DetectLanguage(test.name)
		if got != test.want || ok != test.ok {
			t.Errorf("DetectLanguage(%q) = %q, %t; want %q, %t", test.name, got, ok, test.want, test.ok)
		}
	}
}

func TestLooksLikeJSXOpenBoundsGuard(t *testing.T) {
	if looksLikeJSXOpen(nil, 0, 0) {
		t.Fatal("expected false for empty source")
	}
	if looksLikeJSXOpen([]byte("<"), 0, 1) {
		t.Fatal("expected false for source containing only '<'")
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

func TestExtractShellSkipsQuotesHeredocsAndHereStrings(t *testing.T) {
	source := []byte("#!/usr/bin/env bash\n" +
		"echo \"# not a comment '{}`\" '# also not' ${VAR#pattern} $#\n" +
		"echo foo#bar # a real trailing comment\n" +
		"# full line comment\n" +
		"cat <<'EOF'\n# not a comment (heredoc body)\nEOF\n" +
		"read x <<<\"$out\"\n" +
		"# after here-string\n")
	catalog, err := Extract("sample.sh", Shell, source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := commentTexts(catalog), []string{"# a real trailing comment", "# full line comment", "# after here-string"}; !equalStrings(got, want) {
		t.Fatalf("comments = %#v, want %#v", got, want)
	}
}

func TestExtractShellHeredocForms(t *testing.T) {
	source := []byte("cat <<PLAIN\nbody # not comment\nPLAIN\n" +
		"cat <<'QUOTED'\nbody # not comment\nQUOTED\n" +
		"cat <<\\ESC\nbody # not comment\nESC\n" +
		"cat <<-TABBED\n\tbody # not comment\n   TABBED\n")
	catalog, err := Extract("sample.sh", Shell, source)
	if err != nil {
		t.Fatal(err)
	}
	if got := commentTexts(catalog); len(got) != 0 {
		t.Fatalf("heredoc bodies leaked as comments: %#v", got)
	}
}

func TestExtractShellRejectsUnterminatedLiterals(t *testing.T) {
	for _, bad := range [][]byte{
		[]byte("echo \"unterminated"),
		[]byte("echo 'unterminated"),
		[]byte("cat <<EOF\nbody"),
		[]byte("cat <<\"EOF\""),
	} {
		if _, err := Extract("bad.sh", Shell, bad); err == nil {
			t.Fatalf("unterminated shell literal accepted: %q", bad)
		}
	}
	// A well-formed multi-line single-quote idiom ('foo'\'' bar) is accepted.
	catalog, err := Extract("sample.sh", Shell, []byte("cmd='foo '\\''\nbody\n'\\'' /input'\n# real\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := commentTexts(catalog); !equalStrings(got, []string{"# real"}) {
		t.Fatalf("comments = %#v", got)
	}
}
