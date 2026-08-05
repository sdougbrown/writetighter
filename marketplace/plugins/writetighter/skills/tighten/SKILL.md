---
name: tighten
description: "Check drafted prose with the local writetighter CLI — deterministic `lint` plus contextual `revise` — and apply the findings before shipping. Use when about to post a PR description, a plan, a doc/README section, or a commit body and you want it checked; when asked to 'tighten the comments in this PR'; or when the user says 'tighten this', 'run writetighter', 'check this writing', or 'is this prose tight enough?'. Do NOT use for grammar/spell-checking or for rewriting a file that is not yours — it reports rule findings and model-suggested rewrites, it never auto-fixes."
argument-hint: '[text or file to tighten; optional --kind]'
compatibility: "Requires the writetighter binary on PATH. `revise` also needs a configured OpenAI-compatible model (run `writetighter config`); without one, fall back to subagent delegation."
---

$ARGUMENTS: the prose to tighten (inline text or a file path), plus an optional `--kind`

# Tighten

Check prose you are about to ship, then apply the findings you can verify and
re-check. The profile and dictionary are **embedded in the binary** — you do not
normally pass `--profile` or a dictionary path.

writetighter never edits files. **You** do the revising.

## Resolve the input

Arguments arrive as one flat string; `--kind <value>` can appear anywhere.

1. Set `$KIND` to the explicit `--kind` value when present. Otherwise choose the
   matching kind from the table below based on the requested artifact; use
   `description` when no specialized kind applies. If the value is not listed,
   report it and the valid options, then stop — do not guess a replacement.
2. If the remaining argument names an existing file, read its full contents into
   `$DRAFT`. Otherwise treat it as inline prose. If no prose remains, use the
   draft targeted in the current conversation; ask which draft when ambiguous.
3. Keep `$DRAFT` as the working copy and update it whenever you accept an edit.

## Two commands — run both

`lint` and `revise` are complementary. `revise` runs independently of `lint`
(no lint findings required) and catches the subtle stuff, so **always run it** —
don't stop at a clean `lint`.

| Command | What it catches | Model? |
|---|---|---|
| `lint` | Deterministic profile rules: sentence length, dense paragraphs, noun stacks, gerund openers, discouraged terms, contractions, banned modals, Latin abbreviations. Auditable, fast. | No |
| `revise` | Contextual rewrites and clarification questions for ambiguous referents, indirect voice, combined topics, and missing relationships. | Yes |

## Pick the `--kind`

Sets the `lint` sentence-length limit and gives `revise` context:

| `--kind` | Use for | Sentence limit |
|---|---|---|
| `pr` | PR titles/bodies, commit bodies | 20 words |
| `procedure` | Step-by-step instructions | 20 words |
| `description` | Prose docs, READMEs, plans | 25 words |
| `code-comment` | Inline comments, doc comments, TODOs | 25 (inherits `description`) |
| `reference` | API, configuration, and lookup documentation | 25 (inherits) |
| `decision` | ADRs, design decisions, tradeoff records | 25 (inherits) |
| `incident` | Incident reports, timelines, remediations | 25 (inherits) |
| `agent-instruction` | Skills, system prompts, delegation prompts | 25 (inherits) |
| `status-update` | Progress, delay, blocker, diagnostic updates | 25 (inherits) |

Before the loop, run `command -v writetighter`. If it is missing, tell the user
to install it (`curl -fsSL https://writetighter.douggo.com/install.sh | bash`)
and make sure it is on `PATH`. Do not attempt an unrequested installation.

## Core loop

1. **Lint** (deterministic):

   ```sh
   printf '%s' "$DRAFT" | writetighter lint --stdin --kind "$KIND" --format json
   ```

   Parse every item in `findings[]`; use its `rule_id`, `range`, `evidence`, and
   `message`. A clean result has `status: linted` and an empty `findings[]`. Only
   `CORE.SENTENCE_LENGTH` is enforced; the rest are `candidate` (advisory). If
   lint exits with code 2 or returns no valid JSON, report the failure and stop
   — deterministic verification did not run.

2. **Revise** (contextual — always run this):

   ```sh
   printf '%s' "$DRAFT" | writetighter revise --stdin --kind "$KIND"
   ```

   Output is JSON by default (also accepts `--format human`). Each entry in
   `revisions[]` has:
   - `kind`: `"rewrite"` or `"clarification"`
   - `source_text`: the exact span it addresses
   - `replacement` (rewrite) — suggested tighter text
   - `question` (clarification) — a concrete question, not a guessed rewrite
   - `principle_ids`, `reason`, `confidence` (0–1)
   - top-level `discarded_rewrites` counts model rewrites dropped for losing
     protected content (commands, paths, numbers, defined terms); `errors[]`
     is per-document model failures.

   **If `revise` is unconfigured** (exits with code 3) or you have no model
   endpoint, do not start interactive setup. Fall back to **subagent
   delegation** — see the reference for your host:
   `references/host-claude.md` or `references/host-codex.md`. Continue with the
   lint results meanwhile and report that contextual revision was delegated.

3. **Apply what you can verify:**
   - **`CORE.SENTENCE_LENGTH`** (enforced) — split the over-limit sentence into
     independent claims.
   - **candidate findings** (`DENSE_PARAGRAPH`, `NOUN_STACK`, `GERUND_OPENER`,
     `TERM_DISCOURAGED`, `CONTRACTION`, `BANNED_MODAL`, `LATIN_ABBREV`) — fix
     when they help the reader; they are advisory.
   - **`rewrite`** — adopt `replacement` when it preserves meaning and every
     command/path/identifier/number in `source_text`. Skip low-confidence
     suggestions you cannot confirm.
   - **`clarification`** — answer the `question` by adding the missing fact. If
     you don't know it, surface the question to the user; never invent it.

4. **Update `$DRAFT`** with every accepted change, then **re-lint** until
   `status: linted` with no enforced findings (remaining `candidate` findings are
   fine if intentional). A second `revise` pass is optional once its
   high-confidence items are handled.

5. **Present** the tightened prose. Note any finding left unaddressed and why,
   and relay any clarification question you could not resolve.

## Snippets and PR comments

Never write ephemeral text to a file just to check it — both commands take
`--text "..."` and `--stdin`. Pass the snippet directly; no temp `.md` files.

When the invocation targets **comments introduced in a PR**, derive the added
comment lines from the diff (`git diff`), not whole files. Evaluate each comment
(or contiguous block) on its own with `--kind code-comment`, one `--text` per
comment — don't concatenate unrelated comments, or the source ranges blur. Use
`--kind code-comment` (inherits the 25-word `description` limit); this lens
favors non-obvious constraints, invariants, rationale, and effects over syntax
narration. Relay each `rewrite` `replacement` as a proposed edit for the author
to accept; for a `clarification`, ask the author — don't invent the missing
detail.

## Delegating revision without a model

`writetighter prompt --kind <kind> --format json` prints the same revision
directions `revise` uses, without reading input or requiring a model. When you
have no configured endpoint, pass that output plus the target prose to a
subagent that returns the same `revisions[]` JSON shape. The exact delegation
mechanism depends on your host:

- **Claude Code**: see `references/host-claude.md`
- **Codex**: see `references/host-codex.md`

This does not replace deterministic `lint`, and you still verify and apply
every proposed edit.