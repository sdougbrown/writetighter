# Agent Integration

There's no 3,000-line skill file here. `writetighter` is a CLI; wiring it
into an agent workflow is a handful of shell calls plus a rule for what to do
with the output. Two shapes cover most of what you'll want:

- **Local model** — the agent runs `revise` directly against a model you've
  configured once with `writetighter config`.
- **Delegated model** — no local config at all. The agent runs `writetighter
  prompt` to get the revision instructions, then hands them to a cheap
  subagent that makes its own model call.

Both examples below are illustrative starting points, not a maintainer's
personal dotfiles — adapt the frontmatter to whatever skill or command format
your harness uses. Both follow the same two rules `writetighter` itself
enforces: it never edits files, and a `clarification` gets asked, not
guessed.

## Local model

Use this when the agent already has a configured endpoint (local or hosted)
and can call `revise` directly.

```markdown
---
name: tighten
description: Run drafted prose through writetighter before shipping it — a PR body, doc section, or commit message. Reports findings; you apply them.
argument-hint: '[text or file to tighten; optional --kind]'
---

$ARGUMENTS: the prose to tighten, plus an optional `--kind`

1. Resolve `$KIND` (default `description`) and `$DRAFT` — read the argument
   as a file if it names one that exists, otherwise treat it as inline text.

2. Lint (deterministic, no model):

   \`\`\`sh
   printf '%s' "$DRAFT" | writetighter lint --stdin --kind "$KIND" --format json
   \`\`\`

3. Revise (contextual, always run this too — a clean lint doesn't mean the
   prose is tight):

   \`\`\`sh
   printf '%s' "$DRAFT" | writetighter revise --stdin --kind "$KIND"
   \`\`\`

4. For each `rewrite`, adopt `replacement` only if it preserves every
   command, path, number, and identifier present in `source_text`. For each
   `clarification`, surface the `question` — never invent the answer.

5. Update `$DRAFT` with accepted changes, re-lint, present the result. Name
   anything you left unresolved and why.

Requires `writetighter config` to have run once, interactively, ahead of time.
```

## Delegated model

Use this when the agent has no configured endpoint — or shouldn't be trusted
with one — and a cheap subagent (Haiku-class is plenty) can make the call
instead.

```markdown
---
name: tighten-delegate
description: Tighten drafted prose without a locally configured model. Hands the revision instructions to a cheap subagent that owns its own model call.
argument-hint: '[text or file to tighten; optional --kind]'
---

$ARGUMENTS: the prose to tighten, plus an optional `--kind`

1. Resolve `$KIND` and `$DRAFT` as in the local-model version.

2. Lint locally — this never needs a model either way:

   \`\`\`sh
   printf '%s' "$DRAFT" | writetighter lint --stdin --kind "$KIND" --format json
   \`\`\`

3. Get the revision instructions without calling a model yourself:

   \`\`\`sh
   writetighter prompt --kind "$KIND" --format json
   \`\`\`

4. Spawn a cheap subagent (e.g. Haiku) with two inputs: the prompt text from
   step 3, and `$DRAFT`. Ask it to return the same `revisions[]` shape
   `revise` would — `rewrite` or `clarification` items with `source_text`,
   `reason`, and `confidence`.

5. Apply the subagent's output exactly as step 4 in the local-model
   version — verify protected content survives every `rewrite`, surface every
   `clarification` instead of guessing.

No `writetighter config` needed. The subagent owns its own model call and its
own cost; the primary agent only owns verification.
```

## Which one to use

The local-model version is simpler when you already run a model behind the
scenes for other tasks. The delegated version costs one extra subagent call
but works with zero local setup — useful for a shared skill you don't want to
gate behind "did you run `writetighter config` on this machine."

Both feed the same two commands documented on the [CLI reference](/cli).
Neither one writes to your files; that decision — and the actual edit —
stays with whoever's holding the pen.
