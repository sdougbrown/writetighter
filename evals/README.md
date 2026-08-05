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
same recall denominator. After configuring WT with Gemma's deployed 80k-token
context window, two `gemma4` generation passes scored by local
`nemotron-ultra` produced:

| Variant | Recall | Overall quality | False positives |
|---|---:|---:|---:|
| Production baseline | `0.25`, `0.25` (mean `0.25`) | `35`, `35` | `2`, `1` |
| Conservative whole-file candidate | `0.75`, `0.75` (mean `0.75`) | `45`, `55` | `9`, `7` |

The candidate clears the recall gate and processed every source file. Production
WT still failed four or five files per pass: three larger files exceeded WT's
separate 32,000-character transport ceiling, while other failures came from
model responses whose `source_text` could not be resolved exactly. The
candidate is not ready to ship because precision remains poor despite
`temperature: 0`. A lexer-enforced target set, stronger structured output, and
repeated-trial precision gates are required. The aggressive prompt remains
available for comparison but is not the recommended candidate.

A single exploratory `qwen-moe` pass used
`chat_template_kwargs.enable_thinking: false`; otherwise hidden reasoning
exhausted the 4,096-token response budget on half the corpus. Qwen reached
recall `1.0`, but emitted 35 findings, of which the judge marked 31 as false
positives, for overall quality `35`. The gateway's thinking-off
`qwen-moe-instruct` alias scored production WT at recall `0.25` with five false
positives and the whole-file candidate at recall `1.0` with 24 false positives;
both received overall quality `30`. The candidate emitted 28 findings. Neither
Qwen route is a better candidate than Gemma under this prompt. The driver's
optional `chat_template_kwargs` setting exists for model-specific chat-template
controls and is not enabled in the committed Gemma fixture.

## Scope and caveats

- Corpus languages: Go, TypeScript, Rust, and Python.
- Python docstrings are intentionally outside the comment target set.
- `comment_target` is computed by a small language-aware scanner and is only
  metadata. The judge is explicitly required to verify targets against source.
- The corpus snapshot is local and gitignored; the hash lock makes sibling-repo
  drift visible without committing copied source.
