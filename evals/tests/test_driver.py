import io
import json
import urllib.request

from wt_evals.driver import _chat, _validate_id_response


def test_chat_forwards_template_kwargs(monkeypatch) -> None:
    captured = {}
    envelope = {
        "choices": [{"message": {"content": '{"findings":[]}'}}],
        "usage": {"total_tokens": 12},
    }

    class Response:
        def __enter__(self):
            return io.BytesIO(json.dumps(envelope).encode())

        def __exit__(self, *_args):
            return None

    def fake_urlopen(request: urllib.request.Request, timeout: float):
        captured["payload"] = json.loads(request.data)
        captured["timeout"] = timeout
        return Response()

    monkeypatch.setattr(urllib.request, "urlopen", fake_urlopen)

    result, usage = _chat(
        base_url="http://localhost:4000/v1",
        model="qwen-moe",
        system="system",
        user="user",
        timeout=30,
        chat_template_kwargs={"enable_thinking": False},
    )

    assert result == {"findings": []}
    assert usage == {"total_tokens": 12}
    assert captured["payload"]["chat_template_kwargs"] == {"enable_thinking": False}
    assert captured["timeout"] == 30


def test_id_protocol_rejects_unsafe_targets_and_normalizes_catalog_owner() -> None:
    catalog = {
        "comments": [
            {
                "id": "c0001",
                "form": "line",
                "text": "// original",
                "span": {
                    "start_byte": 10,
                    "end_byte": 21,
                    "start_line": 2,
                    "end_line": 2,
                },
            }
        ]
    }
    response = {
        "findings": [
            {
                "comment_id": "c0001",
                "action": "rewrite",
                "principle_ids": ["CORE.SHORT_SENTENCE"],
                "reason": "The comment is unclear.",
                "replacement": "// clear",
                "question": None,
                "confidence": 0.9,
            },
            {
                "comment_id": "c9999",
                "action": "clarification",
                "principle_ids": ["CORE.SHORT_SENTENCE"],
                "reason": "Unknown target.",
                "question": "What does this mean?",
                "replacement": None,
                "confidence": 0.9,
            },
        ]
    }
    accepted, rejected, low_confidence = _validate_id_response(
        response=response,
        catalog=catalog,
        known_principles={"CORE.SHORT_SENTENCE"},
        replacement_is_safe=lambda replacement, form: replacement == "// clear"
        and form == "line",
        minimum_confidence=0.8,
    )

    assert accepted == [(response["findings"][0], catalog["comments"][0])]
    assert rejected[0]["reason"] == "unknown comment_id"
    assert low_confidence == []


def test_id_protocol_rejects_bad_payloads_and_records_low_confidence() -> None:
    catalog = {
        "comments": [
            {
                "id": "c0001",
                "form": "line",
                "text": "// original",
                "span": {
                    "start_byte": 0,
                    "end_byte": 11,
                    "start_line": 1,
                    "end_line": 1,
                },
            },
            {
                "id": "c0002",
                "form": "line",
                "text": "// second",
                "span": {
                    "start_byte": 12,
                    "end_byte": 21,
                    "start_line": 2,
                    "end_line": 2,
                },
            },
        ]
    }
    findings = [
        {
            "comment_id": "c0001",
            "action": "rewrite",
            "principle_ids": ["UNKNOWN"],
            "reason": "Bad principle.",
            "replacement": "// replacement",
            "question": None,
            "confidence": 0.9,
        },
        {
            "comment_id": "c0002",
            "action": "clarification",
            "principle_ids": ["CORE.SHORT_SENTENCE"],
            "reason": "Too uncertain.",
            "replacement": None,
            "question": "What does this mean?",
            "confidence": 0.7,
        },
    ]
    accepted, rejected, low_confidence = _validate_id_response(
        response={"findings": findings},
        catalog=catalog,
        known_principles={"CORE.SHORT_SENTENCE"},
        replacement_is_safe=lambda *_args: True,
        minimum_confidence=0.8,
    )

    assert accepted == []
    assert rejected[0]["reason"] == "unknown principle_id"
    assert low_confidence[0]["reason"] == "confidence below 0.8"
