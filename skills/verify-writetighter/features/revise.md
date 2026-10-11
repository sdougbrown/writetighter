# Contextual revision (revise / rewrite)

## Sub-features

- `revise` analyzes whole documents (path, `--stdin`, `--text`) and returns
  structured rewrites or clarification questions; never edits the file.
- `revise --kind code-comment` lexes Go/TS/JS/Rust/Python/shell sources;
  only cataloged comments can become findings; rejects `--reference`.
- `revise --reference FILE-or-DIR` adds read-only reference context
  (credential-like files are refused/skipped; context window auto-detected
  from `/v1/models`).
- `rewrite` is the whole-passage variant (also model-backed).
- Protected technical content: rewrites that lose commands/paths/numbers are
  discarded (`discarded_rewrites` / `discarded_findings` counters).
- Model failure surfaces as exit code 3 with per-document `errors`.

## How to get to it (user POV)

The user configures a model once (`writetighter config`), then runs
`writetighter revise doc.md --kind procedure` and reads the JSON/human report
of suggestions. Nothing is written back.

## Driving it with CLI + LLM stub

Start the stub and scratch config exactly as `../SKILL.md` Launch describes
(stub port 8731, `response_mode = "prompt_json"`), then:

```sh
printf 'Restart the service after changing the file.\n' > "$WT/revise-fixture.md"
before=$(sha256sum "$WT/revise-fixture.md" | cut -d' ' -f1)
"$WT/writetighter" revise "$WT/revise-fixture.md" --kind procedure --format json > "$WT/revise.json"
echo exit=$?   # expect 0
after=$(sha256sum "$WT/revise-fixture.md" | cut -d' ' -f1)
[ "$before" = "$after" ] && echo "fixture untouched"
python3 - <<'EOF'
import json
d = json.load(open(f"{__import__('os').environ['WT']}/revise.json"))
r = d["revisions"][0]
assert r["kind"] == "rewrite"
assert r["source_text"] == "Restart the service after changing the file."
assert r["replacement"] and r["confidence"] == 0.9
assert d["analysis"][0]["complete"] is True
EOF
# Clarification path: flip the stub's finding kind (helpers/llm_stub.py CANNED_FINDING)
#   or point the scratch config at a dead port to prove exit 3:
sed -i 's/8731/8799/' "$XDG_CONFIG_HOME/writetighter/config.toml"
"$WT/writetighter" revise --text "Restart the service." --kind procedure --format json
echo exit=$?   # expect 3 (model call failed), JSON has errors[]
```

Observable end state: exit 0, `revisions` with the stub's canned rewrite,
`analysis` showing 1 model request and complete coverage, fixture file
byte-identical before/after. Failure path: exit 3 with `errors` populated.

## Gotchas

- Unconfigured non-interactive runs fail (exit 2) with a `writetighter
  config` hint. Only a run on a real terminal auto-starts the wizard (the
  gate is `stdinIsTerminal()`); `--stdin`, `--text`, and scripted/pipe runs
  never prompt.
- The stub's canned `source_text` must appear verbatim in the fixture;
  mismatch relies on nearest-occurrence fallback and can produce zero
  findings rather than an error.
- The stub never applies anything: the real proof of "no automatic
  application" is the unchanged fixture checksum, not the report content.
- Principle IDs in the stub response must be from the revision allowlist
  (e.g. `CORE.SHORT_SENTENCE`); invalid IDs are rejected and the run fails.
- Model-request budget comes from `max_requests` in user config; large
  fixtures can be chunked (watch `analysis[].model_requests`).
