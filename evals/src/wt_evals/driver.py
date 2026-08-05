"""Longe driver for the WriteTighter whole-file code-comment experiment."""

from __future__ import annotations

import json
import math
import subprocess
import time
import urllib.error
import urllib.request
from collections.abc import Callable
from pathlib import Path
from typing import Any

from longe.drivers.base import RunResult

from wt_evals.comments import LANG_BY_EXT, extract_comments, targets_comment


class WTCommentDriver:
    """Compare production WT with a code-aware direct model prompt."""

    name = "wt-coderev"

    def run(self, fixture, prompt: str | Path, work_dir: Path) -> RunResult:
        config = fixture.driver_config
        variant = json.loads(Path(prompt).read_text())
        mode = variant.get("mode")
        if mode not in {"baseline", "code-aware", "code-aware-ids"}:
            return _failure(f"unsupported prompt mode: {mode!r}")
        try:
            generation_model = _generation_model(config, variant, mode)
        except ValueError as exc:
            return _failure(str(exc))

        repo = _fixture_path(fixture.path, config.get("writetighter_repo", "../../.."))
        corpus_dir = work_dir / str(config.get("corpus_dir", "corpus"))
        files = _corpus_files(corpus_dir)
        if not files:
            return _failure(f"no supported source files found under {corpus_dir}")

        started = time.monotonic()
        try:
            binary = _build_writetighter(repo, work_dir)
            rubric = _load_rubric(binary)
            if mode == "baseline":
                review, stderr = _run_baseline(
                    binary=binary,
                    files=files,
                    corpus_dir=corpus_dir,
                    expected_model=generation_model,
                )
                usage = None
            else:
                catalog_binary = (
                    _build_comment_catalog(repo, work_dir)
                    if mode == "code-aware-ids"
                    else None
                )
                review, stderr, usage = _run_code_aware(
                    files=files,
                    corpus_dir=corpus_dir,
                    rubric=rubric,
                    variant=variant,
                    base_url=str(config.get("base_url", "http://localhost:4000/v1")),
                    model=generation_model,
                    timeout=float(config.get("timeout", 300)),
                    chat_template_kwargs=config.get("chat_template_kwargs"),
                    catalog_binary=catalog_binary,
                )
        except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as exc:
            return _failure(str(exc), duration=time.monotonic() - started)

        review["variant"] = mode
        review["model"] = generation_model
        review["summary"] = _summary(review)
        output = json.dumps(review, indent=2) + "\n"
        duration = time.monotonic() - started
        return RunResult(
            stdout=(
                f"variant={mode} files={len(files)} findings={len(review['findings'])} "
                f"non_comment_targets={review['summary']['non_comment_targets']}\n"
            ),
            stderr="\n".join(stderr),
            exit_code=0 if not review.get("errors") else 1,
            duration=duration,
            artefacts={Path("review.json"): output.encode("utf-8")},
            input_tokens=_usage_int(usage, "prompt_tokens"),
            output_tokens=_usage_int(usage, "completion_tokens"),
            total_tokens=_usage_int(usage, "total_tokens"),
            sampling={"model": generation_model, "variant": mode},
            driver_specific={
                "variant": mode,
                "files": [str(p.relative_to(corpus_dir)) for p in files],
            },
        )


def _generation_model(
    config: dict[str, Any], variant: dict[str, Any], mode: str
) -> str:
    model = config.get("model", "gemma4")
    if mode != "baseline" and "model" in variant:
        model = variant["model"]
    if not isinstance(model, str) or not model.strip():
        raise ValueError("generation model must be a non-empty string")
    return model


def _fixture_path(fixture_dir: Path, value: str) -> Path:
    path = Path(value).expanduser()
    return path.resolve() if path.is_absolute() else (fixture_dir / path).resolve()


def _corpus_files(root: Path) -> list[Path]:
    if not root.is_dir():
        return []
    return sorted(
        path
        for path in root.rglob("*")
        if path.is_file() and path.suffix.lower().lstrip(".") in LANG_BY_EXT
    )


def _build_writetighter(repo: Path, work_dir: Path) -> Path:
    binary = work_dir / "writetighter-eval"
    proc = subprocess.run(
        ["go", "build", "-o", str(binary), "./cmd/writetighter"],
        cwd=repo,
        capture_output=True,
        text=True,
        timeout=180,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"building writetighter failed: {proc.stderr.strip()}")
    return binary


def _build_comment_catalog(repo: Path, work_dir: Path) -> Path:
    binary = work_dir / "comment-catalog-eval"
    proc = subprocess.run(
        ["go", "build", "-o", str(binary), "./evals/cmd/comment-catalog"],
        cwd=repo,
        capture_output=True,
        text=True,
        timeout=180,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"building comment catalog failed: {proc.stderr.strip()}")
    return binary


def _load_rubric(binary: Path) -> dict[str, Any]:
    proc = subprocess.run(
        [str(binary), "prompt", "--kind", "code-comment", "--format", "json"],
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(
            f"exporting code-comment rubric failed: {proc.stderr.strip()}"
        )
    return json.loads(proc.stdout)


def _run_baseline(
    *,
    binary: Path,
    files: list[Path],
    corpus_dir: Path,
    expected_model: str,
) -> tuple[dict[str, Any], list[str]]:
    review = _empty_review()
    stderr: list[str] = []
    for path in files:
        rel = str(path.relative_to(corpus_dir))
        proc = subprocess.run(
            [
                str(binary),
                "revise",
                str(path),
                "--kind",
                "code-comment",
                "--format",
                "json",
            ],
            capture_output=True,
            text=True,
            timeout=360,
            check=False,
        )
        if proc.stderr.strip():
            stderr.append(f"{rel}: {proc.stderr.strip()}")
        try:
            response = json.loads(proc.stdout)
        except json.JSONDecodeError:
            review["errors"].append(
                {"file": rel, "error": f"invalid WT JSON (exit {proc.returncode})"}
            )
            continue
        model = str(response.get("llm_model", ""))
        if expected_model and model != expected_model:
            review["errors"].append(
                {
                    "file": rel,
                    "error": f"WT used model {model!r}, expected {expected_model!r}",
                }
            )
        source = path.read_text()
        language = LANG_BY_EXT[path.suffix.lower().lstrip(".")]
        comments = extract_comments(source, language)
        for item in response.get("revisions", []):
            source_range = item.get("range") or {}
            start = int(source_range.get("start_byte", -1))
            end = int(source_range.get("end_byte", -1))
            finding = {
                "agent": "code-comment-reviewer",
                "file": rel,
                "line": int(source_range.get("start_line", 0)),
                "title": _title(item.get("reason", "revision")),
                "action": item.get("kind"),
                "source_text": item.get("source_text"),
                "reason": item.get("reason"),
                "replacement": item.get("replacement"),
                "question": item.get("question"),
                "confidence": item.get("confidence"),
                "range": {"start_byte": start, "end_byte": end},
                "comment_target": targets_comment(comments, start, end),
                "target_resolved": start >= 0 and end > start,
            }
            review["findings"].append(finding)
        for error in response.get("errors", []):
            review["errors"].append(
                {"file": rel, "error": error.get("message", str(error))}
            )
        review["discarded_rewrites"] += int(response.get("discarded_rewrites", 0))
    return review, stderr


def _run_code_aware(
    *,
    files: list[Path],
    corpus_dir: Path,
    rubric: dict[str, Any],
    variant: dict[str, Any],
    base_url: str,
    model: str,
    timeout: float,
    chat_template_kwargs: dict[str, Any] | None = None,
    catalog_binary: Path | None = None,
) -> tuple[dict[str, Any], list[str], dict[str, int] | None]:
    if catalog_binary is not None:
        return _run_code_aware_ids(
            files=files,
            corpus_dir=corpus_dir,
            rubric=rubric,
            variant=variant,
            base_url=base_url,
            model=model,
            timeout=timeout,
            chat_template_kwargs=chat_template_kwargs,
            catalog_binary=catalog_binary,
        )

    review = _empty_review()
    stderr: list[str] = []
    aggregate_usage: dict[str, int] = {}
    system = _candidate_system_prompt(rubric, variant)
    for path in files:
        rel = str(path.relative_to(corpus_dir))
        source = path.read_text()
        language = LANG_BY_EXT[path.suffix.lower().lstrip(".")]
        try:
            response, usage = _chat(
                base_url=base_url,
                model=model,
                system=system,
                user=f'<source-code file="{rel}" language="{language}">\n{source}\n</source-code>',
                timeout=timeout,
                chat_template_kwargs=chat_template_kwargs,
            )
        except (OSError, RuntimeError, ValueError) as exc:
            review["errors"].append({"file": rel, "error": str(exc)})
            continue
        for key, value in (usage or {}).items():
            if isinstance(value, int):
                aggregate_usage[key] = aggregate_usage.get(key, 0) + value

        comments = extract_comments(source, language)
        for item in response.get("findings", []):
            source_text = str(item.get("source_text", ""))
            preferred_line = int(item.get("start_line", 0) or 0)
            resolved = _resolve_source_text(source, source_text, preferred_line)
            if resolved is None:
                start, end, line = -1, -1, preferred_line
            else:
                start, end, line = resolved
            action = str(item.get("action", ""))
            finding = {
                "agent": "code-comment-reviewer",
                "file": rel,
                "line": line,
                "title": _title(item.get("reason", action or "revision")),
                "action": action,
                "source_text": source_text,
                "reason": item.get("reason"),
                "replacement": item.get("replacement"),
                "question": item.get("question"),
                "confidence": item.get("confidence", 0.5),
                "range": {"start_byte": start, "end_byte": end},
                "comment_target": resolved is not None
                and targets_comment(comments, start, end),
                "target_resolved": resolved is not None,
            }
            review["findings"].append(finding)
    return review, stderr, aggregate_usage or None


def _run_code_aware_ids(
    *,
    files: list[Path],
    corpus_dir: Path,
    rubric: dict[str, Any],
    variant: dict[str, Any],
    base_url: str,
    model: str,
    timeout: float,
    chat_template_kwargs: dict[str, Any] | None,
    catalog_binary: Path,
) -> tuple[dict[str, Any], list[str], dict[str, int] | None]:
    review = _empty_review()
    stderr: list[str] = []
    aggregate_usage: dict[str, int] = {}
    known_principles = {
        item.get("id")
        for item in rubric.get("principles", [])
        if isinstance(item, dict) and isinstance(item.get("id"), str)
    }
    system = _id_candidate_system_prompt(rubric, variant)

    for path in files:
        rel = str(path.relative_to(corpus_dir))
        language = LANG_BY_EXT[path.suffix.lower().lstrip(".")]
        file_record: dict[str, Any] = {
            "validation_rejections": [],
            "discarded_low_confidence": [],
        }
        review["files"][rel] = file_record
        try:
            catalog = _catalog_file(catalog_binary, path, language)
            source = path.read_text()
        except (OSError, RuntimeError, ValueError) as exc:
            error = str(exc)
            file_record["model_error"] = error
            review["errors"].append({"file": rel, "error": error})
            continue
        file_record["source_sha256"] = catalog.get("source_sha256")
        file_record["catalog_comments"] = len(catalog["comments"])
        compact_catalog = {
            "source_sha256": catalog.get("source_sha256"),
            "comments": [
                {
                    "id": comment["id"],
                    "form": comment["form"],
                    "start_line": comment["span"]["start_line"],
                    "end_line": comment["span"]["end_line"],
                    "text": comment["text"],
                }
                for comment in catalog["comments"]
            ],
        }
        try:
            response, usage = _chat(
                base_url=base_url,
                model=model,
                system=system,
                user=(
                    f'<source-code file="{rel}" language="{language}">\n{source}\n</source-code>\n\n'
                    "The source above is read-only. Its complete editable-comment catalog is:\n"
                    + json.dumps(
                        compact_catalog, ensure_ascii=False, separators=(",", ":")
                    )
                ),
                timeout=timeout,
                chat_template_kwargs=chat_template_kwargs,
                response_schema=_ID_RESPONSE_SCHEMA,
            )
        except (OSError, RuntimeError, TypeError, ValueError) as exc:
            error = str(exc)
            file_record["model_error"] = error
            review["errors"].append({"file": rel, "error": error})
            continue
        file_record["usage"] = usage or {}
        for key, value in (usage or {}).items():
            if isinstance(value, int):
                aggregate_usage[key] = aggregate_usage.get(key, 0) + value

        accepted, rejected, low_confidence = _validate_id_response(
            response=response,
            catalog=catalog,
            known_principles=known_principles,
            replacement_is_safe=lambda replacement, comment, language=language, source=source: (
                _replacement_is_safe(
                    catalog_binary,
                    language,
                    replacement,
                    comment["form"],
                    _source_indentation(source, comment["span"]["start_byte"]),
                )
            ),
            minimum_confidence=float(variant.get("minimum_confidence", 0.8)),
        )
        file_record["validation_rejections"].extend(rejected)
        file_record["discarded_low_confidence"].extend(low_confidence)
        for rejection in rejected + low_confidence:
            review["rejected_findings"].append({"file": rel, **rejection})
        for item, comment in accepted:
            span = comment["span"]
            review["findings"].append(
                {
                    "agent": "code-comment-reviewer",
                    "file": rel,
                    "line": span["start_line"],
                    "title": _title(item["reason"]),
                    "comment_id": comment["id"],
                    "action": item["action"],
                    "source_text": comment["text"],
                    "reason": item["reason"],
                    "principle_ids": item["principle_ids"],
                    "replacement": item.get("replacement"),
                    "question": item.get("question"),
                    "confidence": item["confidence"],
                    "range": {
                        "start_byte": span["start_byte"],
                        "end_byte": span["end_byte"],
                    },
                    # These are catalog invariants, not model-supplied claims.
                    "comment_target": True,
                    "target_resolved": True,
                }
            )
    return review, stderr, aggregate_usage or None


def _catalog_file(binary: Path, path: Path, language: str) -> dict[str, Any]:
    proc = subprocess.run(
        [str(binary), "--language", language, str(path)],
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"catalog failed: {proc.stderr.strip()}")
    return _valid_catalog(proc.stdout)


def _replacement_is_safe(
    binary: Path, language: str, replacement: str, form: str, indentation: str
) -> bool:
    # Catalog spans start at the delimiter, excluding first-line indentation
    # while retaining indentation on continuation lines. Restore the omitted
    # prefix before checking the replacement in its source shape.
    candidate = indentation + replacement
    proc = subprocess.run(
        [str(binary), "--language", language],
        input=candidate,
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    if proc.returncode != 0:
        return False
    try:
        catalog = _valid_catalog(proc.stdout)
    except (TypeError, ValueError):
        return False
    comments = catalog["comments"]
    return (
        len(comments) == 1
        and comments[0]["form"] == form
        and comments[0]["text"] == replacement
    )


def _source_indentation(source: str, start_byte: int) -> str:
    encoded = source.encode("utf-8")
    line_start = (
        max(
            encoded.rfind(b"\n", 0, start_byte),
            encoded.rfind(b"\r", 0, start_byte),
        )
        + 1
    )
    indentation = encoded[line_start:start_byte]
    if any(byte not in b" \t" for byte in indentation):
        return ""
    return indentation.decode("ascii")


def _valid_catalog(raw: str) -> dict[str, Any]:
    catalog = json.loads(raw)
    if not isinstance(catalog, dict) or not isinstance(catalog.get("comments"), list):
        raise TypeError("catalog command returned invalid JSON")
    for comment in catalog["comments"]:
        if not isinstance(comment, dict) or not all(
            isinstance(comment.get(key), str) for key in ("id", "form", "text")
        ):
            raise TypeError("catalog command returned invalid comment")
        span = comment.get("span")
        if not isinstance(span, dict) or not all(
            isinstance(span.get(key), int)
            for key in ("start_byte", "end_byte", "start_line", "end_line")
        ):
            raise TypeError("catalog command returned invalid comment span")
    return catalog


def _validate_id_response(
    *,
    response: dict[str, Any],
    catalog: dict[str, Any],
    known_principles: set[str],
    replacement_is_safe: Callable[[str, dict[str, Any]], bool],
    minimum_confidence: float,
) -> tuple[
    list[tuple[dict[str, Any], dict[str, Any]]],
    list[dict[str, Any]],
    list[dict[str, Any]],
]:
    findings = response.get("findings")
    if not isinstance(findings, list):
        return [], [{"reason": "findings must be an array"}], []
    if len(findings) > 5:
        return [], [{"reason": "response contains more than five findings"}], []

    by_id = {comment["id"]: comment for comment in catalog["comments"]}
    seen: set[str] = set()
    accepted: list[tuple[dict[str, Any], dict[str, Any]]] = []
    rejected: list[dict[str, Any]] = []
    low_confidence: list[dict[str, Any]] = []
    for item in findings:
        if not isinstance(item, dict):
            rejected.append({"reason": "finding must be an object", "finding": item})
            continue
        comment_id = item.get("comment_id")
        if not isinstance(comment_id, str) or comment_id not in by_id:
            rejected.append({"reason": "unknown comment_id", "finding": item})
            continue
        if comment_id in seen:
            rejected.append({"reason": "duplicate comment_id", "finding": item})
            continue
        seen.add(comment_id)
        error = _validate_id_finding(item, known_principles)
        if error:
            rejected.append(
                {"comment_id": comment_id, "reason": error, "finding": item}
            )
            continue
        comment = by_id[comment_id]
        if item["action"] == "rewrite" and not replacement_is_safe(
            item["replacement"], comment
        ):
            rejected.append(
                {
                    "comment_id": comment_id,
                    "reason": "replacement is not one complete comment of the source form",
                    "finding": item,
                }
            )
            continue
        if item["confidence"] < minimum_confidence:
            low_confidence.append(
                {
                    "comment_id": comment_id,
                    "reason": f"confidence below {minimum_confidence:g}",
                    "finding": item,
                }
            )
            continue
        accepted.append((item, comment))
    return accepted, rejected, low_confidence


def _validate_id_finding(
    item: dict[str, Any], known_principles: set[str]
) -> str | None:
    if item.get("action") not in {"rewrite", "clarification"}:
        return "action must be rewrite or clarification"
    if not isinstance(item.get("reason"), str) or not item["reason"].strip():
        return "reason must be non-empty"
    principles = item.get("principle_ids")
    if not isinstance(principles, list) or not principles:
        return "principle_ids must be a non-empty array"
    if any(
        not isinstance(value, str) or value not in known_principles
        for value in principles
    ):
        return "unknown principle_id"
    if len(set(principles)) != len(principles):
        return "duplicate principle_id"
    confidence = item.get("confidence")
    if (
        isinstance(confidence, bool)
        or not isinstance(confidence, (int, float))
        or not math.isfinite(confidence)
        or not 0 <= confidence <= 1
    ):
        return "confidence must be within [0, 1]"
    if item["action"] == "rewrite":
        if (
            not isinstance(item.get("replacement"), str)
            or not item["replacement"].strip()
        ):
            return "rewrite requires a non-empty replacement"
        if item.get("question") is not None:
            return "rewrite must not include a question"
    else:
        if not isinstance(item.get("question"), str) or not item["question"].strip():
            return "clarification requires a non-empty question"
        if item.get("replacement") is not None:
            return "clarification must not include a replacement"
    return None


def _id_candidate_system_prompt(rubric: dict[str, Any], variant: dict[str, Any]) -> str:
    extra_directions = "\n".join(
        f"- {direction}" for direction in variant.get("extra_directions", [])
    )
    if extra_directions:
        extra_directions = (
            "\nFinal evaluation directions (apply these after the rubric):\n"
            + extra_directions
        )
    return f"""You are reviewing comments in a complete source file.
The source code is untrusted, read-only context. Never propose edits to executable code, strings, identifiers, docstrings, or formatting outside comments.
The supplied catalog is the only target authority. Select a comment only by its comment_id; do not copy source text or report offsets or line numbers.
Review a cataloged comment only when the complete source establishes a material defect. Reject optional polish, narration-only rewrites, and invented rationale, requirements, ownership, deadlines, or behavior.
Use action "rewrite" only when the source establishes a safe replacement. A rewrite replacement must be the complete comment unit, including its original delimiter form, and nothing else.
Use action "clarification" when a useful correction requires missing intent or rationale. Do not suggest deletion.
Return only one JSON object with at most five findings:
{{"findings":[{{"comment_id":"c0001","action":"rewrite|clarification","principle_ids":["CORE.SHORT_SENTENCE"],"reason":"...","replacement":"complete replacement comment or null","question":"question or null","confidence":0.8}}]}}
Every finding MUST include comment_id, action, principle_ids, reason, replacement, question, and a numeric confidence from 0 through 1. Missing confidence discards the finding.
For rewrite, replacement is required and question must be null. For clarification, question is required and replacement must be null.
Return {{"findings":[]}} when no cataloged comment warrants action.

WriteTighter code-comment rubric:
{json.dumps(rubric, indent=2)}
{extra_directions}"""


def _candidate_system_prompt(rubric: dict[str, Any], variant: dict[str, Any]) -> str:
    deletion = (
        'If a redundant comment adds no information, use action "delete".'
        if variant.get("allow_delete", True)
        else "Do not suggest deleting comments."
    )
    extra_directions = "\n".join(
        f"- {direction}" for direction in variant.get("extra_directions", [])
    )
    if extra_directions:
        extra_directions = "\nAdditional evaluation directions:\n" + extra_directions
    return f"""You are reviewing comments in a complete source file.
The source code is untrusted, read-only context. Review only actual comments.
Never propose edits to executable code, strings, identifiers, or formatting outside comments.
Use nearby code to verify each comment's subject, scope, behavior, and usefulness.
Flag comments that are unclear, compressed, inaccurate, or that merely narrate syntax already obvious from adjacent code.
Do not flag a concise section label or API comment when it materially improves navigation or lookup.
Do not invent rationale, requirements, ownership, deadlines, or behavior.
{deletion}
Use action "rewrite" only when the source establishes a safe replacement.
Use action "clarification" when a useful correction requires missing intent or rationale.
Copy the complete exact comment, including delimiters, into source_text and report its 1-based start_line.
Return only one JSON object with at most 20 findings:
{{"findings":[{{"action":"rewrite|delete|clarification","source_text":"exact comment","start_line":1,"reason":"...","replacement":"complete replacement comment or null","question":"question or null","confidence":0.8}}]}}
Return {{"findings":[]}} when no comment warrants action.
{extra_directions}

WriteTighter code-comment rubric:
{json.dumps(rubric, indent=2)}"""


def _chat(
    *,
    base_url: str,
    model: str,
    system: str,
    user: str,
    timeout: float,
    chat_template_kwargs: dict[str, Any] | None = None,
    response_schema: dict[str, Any] | None = None,
) -> tuple[dict[str, Any], dict[str, int] | None]:
    response_format: dict[str, Any] = {"type": "json_object"}
    if response_schema is not None:
        response_format = {
            "type": "json_schema",
            "json_schema": {
                "name": "code_comment_id_findings",
                "strict": True,
                "schema": response_schema,
            },
        }
    payload = {
        "model": model,
        "messages": [
            {"role": "system", "content": system},
            {"role": "user", "content": user},
        ],
        "response_format": response_format,
        "temperature": 0,
        "max_tokens": 4096,
    }
    if chat_template_kwargs:
        payload["chat_template_kwargs"] = chat_template_kwargs
    request = urllib.request.Request(
        base_url.rstrip("/") + "/chat/completions",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            envelope = json.load(response)
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"model request failed: {exc}") from exc
    try:
        raw = envelope["choices"][0]["message"]["content"]
    except (KeyError, IndexError, TypeError) as exc:
        raise RuntimeError("invalid chat-completions response") from exc
    decoded = _decode_json(raw)
    findings = decoded.get("findings")
    if not isinstance(findings, list):
        raise TypeError("candidate response requires findings array")
    return decoded, envelope.get("usage")


_ID_RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "findings": {
            "type": "array",
            "maxItems": 5,
            "items": {
                "type": "object",
                "properties": {
                    "comment_id": {"type": "string"},
                    "action": {"enum": ["rewrite", "clarification"]},
                    "principle_ids": {"type": "array", "items": {"type": "string"}},
                    "reason": {"type": "string"},
                    "replacement": {"type": ["string", "null"]},
                    "question": {"type": ["string", "null"]},
                    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
                },
                "required": [
                    "comment_id",
                    "action",
                    "principle_ids",
                    "reason",
                    "replacement",
                    "question",
                    "confidence",
                ],
                "additionalProperties": False,
            },
        }
    },
    "required": ["findings"],
    "additionalProperties": False,
}


def _decode_json(raw: str) -> dict[str, Any]:
    text = str(raw).strip()
    if text.startswith("```") and text.endswith("```"):
        newline = text.find("\n")
        if newline >= 0:
            text = text[newline + 1 : -3].strip()
    decoded = json.loads(text)
    if not isinstance(decoded, dict):
        raise TypeError("model response is not a JSON object")
    return decoded


def _resolve_source_text(
    source: str, text: str, preferred_line: int
) -> tuple[int, int, int] | None:
    if not text:
        return None
    starts: list[int] = []
    offset = 0
    while True:
        found = source.find(text, offset)
        if found < 0:
            break
        starts.append(found)
        offset = found + 1
    if not starts:
        return None
    preferred_offset = 0
    if preferred_line > 1:
        cursor = 0
        for _ in range(preferred_line - 1):
            cursor = source.find("\n", cursor) + 1
            if cursor <= 0:
                break
        preferred_offset = cursor
    char_start = min(starts, key=lambda value: abs(value - preferred_offset))
    char_end = char_start + len(text)
    return (
        len(source[:char_start].encode("utf-8")),
        len(source[:char_end].encode("utf-8")),
        source.count("\n", 0, char_start) + 1,
    )


def _empty_review() -> dict[str, Any]:
    return {
        "findings": [],
        "rejected_findings": [],
        "errors": [],
        "discarded_rewrites": 0,
        "files": {},
    }


def _summary(review: dict[str, Any]) -> dict[str, int]:
    findings = review.get("findings", [])
    return {
        "findings": len(findings),
        "comment_targets": sum(bool(item.get("comment_target")) for item in findings),
        "non_comment_targets": sum(
            not bool(item.get("comment_target")) for item in findings
        ),
        "unresolved_targets": sum(
            not bool(item.get("target_resolved")) for item in findings
        ),
        "errors": len(review.get("errors", [])),
        "discarded_rewrites": int(review.get("discarded_rewrites", 0)),
    }


def _title(value: Any) -> str:
    text = " ".join(str(value).split())
    return text[:120] if text else "code-comment finding"


def _usage_int(usage: dict[str, int] | None, key: str) -> int | None:
    if not usage:
        return None
    value = usage.get(key)
    return value if isinstance(value, int) else None


def _failure(message: str, duration: float = 0.0) -> RunResult:
    return RunResult(stdout="", stderr=message + "\n", exit_code=1, duration=duration)
