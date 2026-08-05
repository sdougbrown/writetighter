import io
import json
import urllib.request

from wt_evals.driver import _chat, _generation_model


def test_candidate_can_override_generation_model() -> None:
    config = {"model": "gemma4"}
    assert (
        _generation_model(config, {"model": "qwen-moe-instruct"}, "code-aware-ids")
        == "qwen-moe-instruct"
    )
    assert (
        _generation_model(config, {"model": "qwen-moe-instruct"}, "baseline")
        == "gemma4"
    )


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
