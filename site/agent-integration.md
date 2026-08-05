# Agent Integration

Wiring `writetighter` into an agent workflow requires a small skill file to handle the shell calls and output rules. Use one of these two patterns:

- **Local model** — the agent runs `revise` directly against a configured endpoint.
- **Delegated model** — the agent runs `prompt` to get revision instructions, then hands them to a subagent.

Both examples below are templates. Adapt the frontmatter and command structure to your specific agent harness. Both follow the core `writetighter` rules: it never edits files, and `clarification` questions are asked, never guessed.

## Local model

Use this when your agent already has a configured model endpoint and can call `revise` directly.

```markdown
---
name: tighten
description: Run drafted prose through writetighter before shipping. Reports findings; you apply them.
argument-hint: '[text or file to tighten; optional --kind]'
---

$ARGUMENTS: the prose to tighten, plus an optional `--kind`

1. Resolve `$KIND` (default `description`) and `$DRAFT`.

2. Lint (deterministic):

   `printf '%s' "$DRAFT" | writetighter lint --stdin --kind "$KIND" --format json`

3. Revise (contextual):

   `printf '%s' "$DRAFT" | writetighter revise --stdin --kind "$KIND"`

4. Process revisions:
   - For each `rewrite`: Adopt `replacement` only if it preserves all commands, paths, numbers, and identifiers in `source_text`.
   - For each `clarification`: Surface the `question`. Never guess.

5. Update `$DRAFT` with accepted changes, re-lint, and present the result.
```

## Delegated model

Use this when the agent has no configured endpoint, or when you want to delegate the model call to a cheaper, specialized subagent.

```markdown
---
name: tighten-delegate
description: Tighten prose by delegating revision instructions to a subagent.
argument-hint: '[text or file to tighten; optional --kind]'
---

$ARGUMENTS: the prose to tighten, plus an optional `--kind`

1. Resolve `$KIND` and `$DRAFT` as in the local-model version.

2. Lint locally:

   `printf '%s' "$DRAFT" | writetighter lint --stdin --kind "$KIND" --format json`

3. Get revision instructions:

   `writetighter prompt --kind "$KIND" --format json`

4. Spawn a subagent (e.g. Haiku) with the prompt text and `$DRAFT`. Instruct it to return the same `revisions[]` JSON shape as `revise`.

5. Apply the subagent's output following the same verification rules as the local-model version.
```

## Which one to use?

- **Local model:** Use it if you got it!
- **Cloud model:** Use with an api key if you like. No need for a pricey one!
- **Delegated model:** Good if you've just got a "coding plan" and don't want to configure any API stuff.

Both rely on the commands documented in the [CLI reference](/cli).

::: tip Packaged plugin
For Claude Code or Codex, you can skip the manual skill file and install the bundled `tighten` plugin instead — see the [Claude Code plugin](/claude-plugin) and [Codex plugin](/codex-plugin) guides. The plugin formalizes the workflow below, including subagent delegation when `revise` is unconfigured.
:::
