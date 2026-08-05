# WriteTighter code-comment evaluation

This consumer-owned [Longe](https://github.com/sdougbrown/longe) driver compares two paths over the
same multilingual source corpus and `gemma4` endpoint:

1. **baseline** — production `writetighter revise --kind code-comment` against
   each complete source file;
2. **code-aware** — the exported WriteTighter rubric plus explicit read-only
   source context, comments-only targeting, and deletion support.

Both paths normalize findings into `review.json`. Longe's `gist` evaluator uses
`nemotron-ultra` on the local OpenAI-compatible endpoint to score validity,
false positives, estimated recall, target isolation, actionability, replacement
safety, and overall quality.

## Setup

From the WriteTighter repository:

```sh
python3 evals/scripts/snapshot_corpus.py --force
```

The script copies pinned files from WriteTighter, Avenor, Umpire, Daywatch,
Longe, and vLLM sibling repositories into a gitignored fixture seed and writes
`evals/fixtures/code-comments/corpus.lock.json`. Verify drift later with:

```sh
python3 evals/scripts/snapshot_corpus.py --check
```

Install the consumer driver into Longe's environment:

```sh
cd ~/Code/longe
uv sync
uv pip install -e ~/Code/writetighter/evals
uv run longe drivers list
```

`wt-coderev` should appear as an entry-point driver.

## Run and compare

```sh
cd ~/Code/longe

uv run longe run ~/Code/writetighter/evals/fixtures/code-comments \
  --prompt ~/Code/writetighter/evals/fixtures/code-comments/prompts/baseline.json \
  --no-cache

uv run longe run ~/Code/writetighter/evals/fixtures/code-comments \
  --prompt ~/Code/writetighter/evals/fixtures/code-comments/prompts/code-aware-conservative.json \
  --no-cache

uv run longe line
uv run longe diff <baseline-run-id> <candidate-run-id>
```

The driver builds WriteTighter from the current checkout. Production behavior
is not modified by this evaluation package. `code-aware.json` keeps the more
aggressive initial prompt for recall/precision comparisons;
`code-aware-conservative.json` adds the precision-oriented directions used by
the recommended candidate run.

### Initial result

The fixture includes a reviewed four-finding ground truth, so every run uses the
same recall denominator. Across two `gemma4` generation passes scored by local
`nemotron-ultra`:

| Variant | Recall | Overall quality | False positives |
|---|---:|---:|---:|
| Production baseline | `0.00`, `0.50` (mean `0.25`) | `15`, `45` | `1`, `3` |
| Conservative whole-file candidate | `0.75`, `0.75` (mean `0.75`) | `35`, `78` | `16`, `1` |

The candidate produced higher recall and processed every source file. However,
these results are provisional: the production runs used a local WT config that
did not declare the deployed Gemma model's 80k-token context window. That
configuration mismatch likely contributed to WT's model-budget failures on four
or five of eight files per pass, so the recall gate must be rerun after setting
`llm.context_window_tokens = 80000`. The candidate is also not ready to ship:
its false-positive count varied from one to sixteen despite `temperature: 0`.
A lexer-enforced target set, stronger structured output, and repeated-trial
precision gates are required. The aggressive prompt remains available for
comparison but is not the recommended candidate.

## Scope and caveats

- Corpus languages: Go, TypeScript, Rust, and Python.
- Python docstrings are intentionally outside the comment target set.
- `comment_target` is computed by a small language-aware scanner and is only
  metadata. The judge is explicitly required to verify targets against source.
- The corpus snapshot is local and gitignored; the hash lock makes sibling-repo
  drift visible without committing copied source.
