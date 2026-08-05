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


def test_block_comment_is_one_span() -> None:
    source = "fn main() { /* first\nsecond */ let x = 1; }\n"
    comments = extract_comments(source, "rust")
    assert len(comments) == 1
    assert comments[0].text == "/* first\nsecond */"
    assert comments[0].start_line == 1
    assert comments[0].end_line == 2
