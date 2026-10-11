# Revision guidance export (prompt)

## Sub-features

- `prompt --kind <kind>` prints the core + kind-specific revision guidance
  used by `revise`, for one of nine kinds (`description`, `procedure`, `pr`,
  `code-comment`, `reference`, `decision`, `incident`, `agent-instruction`,
  `status-update`).
- `--format json` returns a versioned JSON structure instead of prose.
- Requires no model, no config, no input document.

## How to get to it (user POV)

The user runs `writetighter prompt --kind code-comment` (or pipes the JSON to
a subagent) to get the same lens `revise` would apply, without a model.

## Driving it with CLI

```sh
"$WT/writetighter" prompt --kind procedure > "$WT/prompt.txt"
echo exit=$?   # expect 0
grep -q "Document kind: procedure" "$WT/prompt.txt"
grep -q "CORE.CAUSAL_ORDER" "$WT/prompt.txt"          # kind lens present
grep -q "CORE.SHORT_SENTENCE" "$WT/prompt.txt"        # core principles present
"$WT/writetighter" prompt --kind decision --format json > "$WT/prompt.json"
python3 -c "import json;d=json.load(open('$WT/prompt.json'));assert d['kind']=='decision'"  # adjust key after first run; schema is versioned JSON
"$WT/writetighter" prompt --kind bogus; echo exit=$?   # expect 2 (unknown kind)
```

Observable end state: prose output contains the "Document kind:" line and
both core and kind-specific principle IDs; `--format json` parses and
identifies the kind; an unknown kind exits 2.

## Gotchas

- The exact JSON field names are versioned — inspect the first run's output
  before asserting deep structure.
- `prompt` must not require model config: it succeeds with an empty scratch
  `XDG_CONFIG_HOME`, which is itself a proof of the "no model access" claim.
- Output is guidance only; nothing here implies findings.
