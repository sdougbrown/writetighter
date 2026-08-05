"""Comment extraction and finding-target classification per language.

The eval measures whether a model finding hits comment text. That requires a
deterministic, language-aware notion of "comment span". This module provides a
small linear scanner per language that:

  * recognizes line comments (``//`` for go/rust/ts, ``#`` for py),
  * recognizes block comments (``/* ... */`` for go/rust/ts),
  * does NOT treat comment-looking text inside string/char literals or
    template/raw strings as comments (e.g. ``url := "http://x/y"``),
  * reports byte ranges, 1-based line numbers, and the comment text.

Python docstrings (triple-quoted strings) are deliberately treated as strings,
not comments, so a model that targets a docstring is flagged as a non-comment
target. This is a heuristic, and per-file numbers feed *estimated* metrics only.
"""

from __future__ import annotations

import bisect
from dataclasses import dataclass

# Language -> canonical extension. Kept here so driver, corpus script, and tests
# agree on the surface the eval understands.
LANGUAGES: dict[str, str] = {
    "go": "go",
    "ts": "ts",
    "rust": "rs",
    "py": "py",
}
LANG_BY_EXT: dict[str, str] = {ext: lang for lang, ext in LANGUAGES.items()}

_LINE_COMMENT = {"go": "//", "rust": "//", "ts": "//", "py": "#"}
_BLOCK_COMMENT = {
    "go": ("/*", "*/"),
    "rust": ("/*", "*/"),
    "ts": ("/*", "*/"),
    "py": None,
}
# Quote chars that may open a literal. ' and " (and sometimes `) are checked in
# scanner order; py strings accept both ' and ", go/rust ' is a char literal,
# ` is a raw string (go/rune, ts template), and rust strings are "..." only.
_STRING_QUOTES = {
    "go": {'"', "`"},
    "rust": {'"'},
    "ts": {'"', "`"},
    "py": {'"', "'"},
}
_CHAR_QUOTES = {"go": "'", "rust": "'", "ts": "'"}
_RAW_QUOTES = {"go": "`", "ts": "`"}


@dataclass(frozen=True)
class Comment:
    """A single comment span in source text.

    ``start``/``end`` are half-open byte offsets into the original text.
    """

    start: int
    end: int
    start_line: int
    end_line: int
    text: str


def extract_comments(text: str, language: str) -> list[Comment]:
    """Return comment spans for *text* in *language* (deterministic order)."""
    if language not in _LINE_COMMENT:
        raise ValueError(f"unsupported comment language: {language!r}")

    line_starts = _line_starts(text)
    line_marker = _LINE_COMMENT[language]
    block = _BLOCK_COMMENT[language]

    n = len(text)
    i = 0
    comments: list[Comment] = []

    # mode: "line" | "block" | "string" | "char" | "raw" | "triple" | None
    mode: str | None = None
    mode_start = 0
    mode_open: str | None = None  # the exact delimiter that opened a literal
    escape = False

    while i < n:
        c = text[i]

        if mode == "line":
            end = text.find("\n", i)
            if end == -1:
                end = n
            comments.append(_mk_comment(text, mode_start, end, line_starts))
            mode = None
            i = end
            continue

        if mode == "block":
            assert block is not None
            close = text.find(block[1], i)
            close = n if close == -1 else close + len(block[1])
            comments.append(_mk_comment(text, mode_start, close, line_starts))
            mode = None
            i = close
            continue

        if mode in ("string", "char", "triple", "raw"):
            if mode == "raw":
                if c == mode_open:
                    mode = None
                i += 1
                continue
            if mode == "triple":
                if text.startswith(mode_open, i):  # type: ignore[arg-type]
                    mode = None
                    i += len(mode_open or "")
                    continue
                i += 1
                continue
            # string / char
            if escape:
                escape = False
                i += 1
                continue
            if c == "\\":
                escape = True
                i += 1
                continue
            if c == mode_open:
                mode = None
            i += 1
            continue

        # NORMAL mode.
        if language in ("go", "rust", "ts"):
            if text.startswith(line_marker, i):
                mode, mode_start = "line", i
                continue
            if block and text.startswith(block[0], i):
                mode, mode_start = "block", i
                i += len(block[0])
                continue
        else:  # python
            if c == "#":
                mode, mode_start = "line", i
                continue

        # Literal detection.
        if language == "py" and _triple_at(text, i) is not None:
            mode, mode_start, mode_open = "triple", i, _triple_at(text, i)
            i += len(mode_open)
            continue
        if c in _RAW_QUOTES.get(language, ()):
            mode, mode_start, mode_open = "raw", i, c
            i += 1
            continue
        if c in _CHAR_QUOTES.get(language, ()):
            mode, mode_start, mode_open = "char", i, c
            i += 1
            continue
        if c in _STRING_QUOTES.get(language, ()):
            mode, mode_start, mode_open = "string", i, c
            i += 1
            continue

        i += 1

    return comments


# --------------------------------------------------------------------------
# helpers
# --------------------------------------------------------------------------


def _triple_at(text: str, i: int) -> str | None:
    if text.startswith('"""', i):
        return '"""'
    if text.startswith("'''", i):
        return "'''"
    return None


def _line_starts(text: str) -> list[int]:
    starts = [0]
    for i, ch in enumerate(text):
        if ch == "\n":
            starts.append(i + 1)
    return starts


def _line_of(starts: list[int], offset: int) -> int:
    return bisect.bisect_right(starts, offset)


def _mk_comment(text: str, start: int, end: int, line_starts: list[int]) -> Comment:
    """Build a comment with UTF-8 byte offsets from scanner character offsets."""
    last = end - 1 if end > start else start
    return Comment(
        start=len(text[:start].encode("utf-8")),
        end=len(text[:end].encode("utf-8")),
        start_line=_line_of(line_starts, start),
        end_line=_line_of(line_starts, last),
        text=text[start:end],
    )


def target_coverage(comments: list[Comment], start: int, end: int) -> float:
    """Fraction of the byte span [start, end) that lies inside a comment.

    Overlapping comment spans are unioned; a target spanning the boundary of a
    block comment and code is reported as partially covered.
    """
    if end <= start:
        return 0.0
    covered = 0
    for c in comments:
        lo = max(start, c.start)
        hi = min(end, c.end)
        if hi > lo:
            covered += hi - lo
    return min(1.0, covered / (end - start))


def targets_comment(
    comments: list[Comment], start: int, end: int, threshold: float = 0.5
) -> bool:
    """True when the majority of the target span lies inside a comment span."""
    return target_coverage(comments, start, end) >= threshold


def addresses_comment(comments: list[Comment], start: int, end: int) -> bool:
    """True when the target overlaps any comment (used for recall estimation)."""
    return any(start < c.end and c.start < end for c in comments)
