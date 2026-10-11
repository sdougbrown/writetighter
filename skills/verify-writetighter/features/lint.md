# Deterministic lint

## Sub-features

- Sentence/paragraph/profile rule findings (`CORE.SENTENCE_LENGTH` enforced;
  candidate rules: dense paragraph, noun stack, gerund opener, contractions,
  banned modals, Latin abbreviations, time anchors, exclamation points,
  number/date style, heading style, list style).
- Input modes: file paths, directories, `--stdin`, `--text` (mutually
  exclusive).
- Output formats: `human` (default), `json`, `agent`.
- Failure threshold: `--fail-on none|warning|error` drives exit codes.
- `--kind code-comment` lexes Go/TS/JS/Rust/Python/shell sources so rules
  inspect only cataloged comments.
- Project config discovery: `.writetighter.toml` found by walking upward from
  cwd; `--config PATH` bypasses discovery.

## How to get to it (user POV)

The user runs `writetighter lint <path-or-stdin> --kind <kind>` and reads a
findings report; exit code tells CI whether the threshold was reached.

## Driving it with CLI

```sh
export XDG_CONFIG_HOME="$WT/cfg"
# Findings + threshold exit code. The sentence must exceed the kind's limit
# (description: 25 words in profile 0.6.0) to trip CORE.SENTENCE_LENGTH:
printf 'This single overly long demonstration sentence deliberately packs far more words than the description profile permits because it keeps going well past the twenty-five word threshold with clause after clause appended.\n' > "$WT/fixture.md"
"$WT/writetighter" lint "$WT/fixture.md" --kind description --format json --fail-on warning
echo exit=$?   # expect 1
python3 -c "import json,sys;d=json.load(open('$WT/out.json'));assert any(f['rule_id']=='CORE.SENTENCE_LENGTH' for f in d['findings'])"  # after redirecting > "$WT/out.json"
# Clean input exits 0 even with --fail-on warning:
printf 'Run the tests.\n' | "$WT/writetighter" lint --stdin --kind pr --fail-on warning
echo exit=$?   # expect 0
# Agent format is line-oriented:
"$WT/writetighter" lint "$WT/fixture.md" --kind description --format agent
# code-comment mode lints only comments in a source file:
printf 'package main\n\n// This single comment deliberately packs far more words than the profile permits and continues well past twenty words.\nfunc main() {}\n' > "$WT/c.go"
"$WT/writetighter" lint "$WT/c.go" --kind code-comment --format agent --fail-on warning
echo exit=$?   # expect 1 with a comment-range finding
```

Observable end state: JSON `findings` array contains the expected `rule_id`
with a byte/line `range` pointing at the fixture text; `--fail-on warning`
flips exit code 1 only when findings exist; clean input exits 0.

## Gotchas

- `--text`, `--stdin`, and paths are mutually exclusive; combining them
  exits 2 with a usage message.
- Exit 1 means the threshold was reached — not an error. Exit 2 is
  usage/config/profile/input failure.
- `--fail-on` defaults to `none` (always exit 0): a missing `--fail-on`
  makes a "failing" proof look green. Always pass it explicitly in proofs.
- The sentence limit differs by `--kind` (`pr`/`procedure`: 20 words,
  `description`: 25 in profile 0.6.0) — a fixture over one limit may be under
  another.
- `code-comment` mode ignores non-comment code; prose in strings and
  identifiers never produces findings.
