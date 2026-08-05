from wt_evals.comments import extract_comments, target_coverage


def test_go_ignores_comment_markers_in_strings_and_uses_utf8_byte_offsets() -> None:
    source = 'package p\nvar u = "https://example.test/x"\n// café rationale\n'
    comments = extract_comments(source, "go")
    assert len(comments) == 1
    comment = comments[0]
    assert comment.text == "// café rationale"
    assert comment.start == len(source[: source.index("// café")].encode("utf-8"))
    assert comment.end == comment.start + len(comment.text.encode("utf-8"))
    assert target_coverage(comments, comment.start, comment.end) == 1.0


def test_typescript_ignores_template_literal_comment_markers() -> None:
    source = "const x = `https://example.test/a`; // actual comment\n"
    comments = extract_comments(source, "ts")
    assert [comment.text for comment in comments] == ["// actual comment"]


def test_python_docstring_is_not_a_comment() -> None:
    source = '"""module documentation"""\n# actual comment\n'
    comments = extract_comments(source, "py")
    assert [comment.text for comment in comments] == ["# actual comment"]


def test_rust_nested_block_comment_is_one_outer_span() -> None:
    source = "fn main() { /* first\n/* nested */\nsecond */ let x = 1; }\n"
    comments = extract_comments(source, "rust")
    assert len(comments) == 1
    assert comments[0].text == "/* first\n/* nested */\nsecond */"
    assert comments[0].start_line == 1
    assert comments[0].end_line == 3


def test_rust_ignores_comment_markers_in_raw_and_byte_raw_strings() -> None:
    source = (
        'let plain = r"// not\n/* not */";\n'
        'let hashes = r###"quote " // not /* not */ "## still raw"###;\n'
        'let byte = br##"quote " // not /* not */ "# still raw"##;\n'
        'let c = cr##"// not /* not */"##;\n'
        "// actual comment\n"
    )
    comments = extract_comments(source, "rust")
    assert [comment.text for comment in comments] == ["// actual comment"]


def test_rust_lifetimes_labels_and_character_literals_resume_scanning() -> None:
    source = (
        "fn f<'a>(value: &'static str) {\n"
        "    'retry: loop {\n"
        "        let slash = '/';\n"
        "        let quote = '\\'';\n"
        "        break 'retry;\n"
        "    }\n"
        "    /* block comment */\n"
        "    // line comment\n"
        "}\n"
    )
    comments = extract_comments(source, "rust")
    assert [comment.text for comment in comments] == [
        "/* block comment */",
        "// line comment",
    ]
