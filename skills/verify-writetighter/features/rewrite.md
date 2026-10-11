# Whole-passage rewrite (rewrite)

## Sub-features

- `rewrite` rewrites one passage in context (model-backed); requires config.
- Flags: `--stdin`, `--text`, `--kind` (default `description`), `--profile`,
  `--config`, `--format` (default `text`), `--model` (override for this run).
- Model failure surfaces as exit code 3.

## How to get to it (user POV)

The user configures a model once (`writetighter config`), then runs
`writetighter rewrite passage.md --kind description` and reads the
human-readable rewrite. One passage is rewritten, not per-range
suggestions.

## Driving it with CLI + LLM stub

Start the stub and scratch config exactly as `../SKILL.md` Launch describes
(stub port 8731, `response_mode = "prompt_json"`), then:

```sh
printf 'Restart the service after changing the file.\n' > "$WT/rewrite-fixture.md"
"$WT/writetighter" rewrite "$WT/rewrite-fixture.md" --kind description
echo exit=$?   # expect 0; prints the rewrite payload (with the stub: the raw findings JSON)
```

## Gotchas

- The stub returns a `findings` envelope shaped for `revise`; `rewrite` uses
  its own response schema. If the stub's canned payload does not satisfy it,
  update `helpers/llm_stub.py` in the same change (per `../SKILL.md`
  maintenance). Verify the actual output on first run before asserting on
  content.
- `rewrite` requires valid model config; unconfigured non-interactive runs
  fail (exit 2) with a `writetighter config` hint.
