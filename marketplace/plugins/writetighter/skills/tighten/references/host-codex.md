# Delegating revision on Codex

Use this path when `writetighter revise` is unconfigured (exits with code 3) or
no model endpoint is available. Codex does not expose a nested subagent with a
separate model the way Claude Code's Agent tool does, so delegation here means
**you produce the structured revisions yourself**, using the `writetighter
prompt` rubric, and hand the result to the core loop.

## The contract

You must emit JSON matching the `revise` output shape: an array of `revisions[]`,
each with `kind` (`"rewrite"` or `"clarification"`), `source_text`, `replacement`
(for rewrites) or `question` (for clarifications), and optionally `principle_ids`,
`reason`, `confidence` (0–1). Treat that shape as the handoff to the core loop's
verification step — do not hand back prose.

## Steps

1. Get the rubric the configured `revise` would have used:

   ```sh
   writetighter prompt --kind "$KIND" --format json
   ```

   This prints the kind-specific revision directions without reading input or
   requiring a model. Read it before revising.

2. Apply that rubric to `$DRAFT` yourself. You are the model in this path, so
   produce the contextual rewrites and clarifications the rubric asks for. Follow
   the same hard rules `revise` enforces:
   - A `replacement` must preserve every command, path, identifier, and number in
     `source_text`. Drop any rewrite that would lose protected content.
   - A `clarification` is a concrete question about a missing fact — never a
     guessed rewrite.
   - Prefer fewer, high-confidence revisions over many speculative ones.

3. Emit the result as `revisions[]` JSON in the shape above and pass it to the
   core loop at **step 3** (apply what you can verify). The verification rules
   still apply — confirm each `replacement` preserves meaning and protected
   content before adopting it, and surface any `clarification` you cannot
   resolve rather than inventing the answer.

## When not to delegate

- The user has a configured endpoint — use `revise` directly. Running
  `writetighter config` once lets `revise` call a dedicated model natively, which
  is preferable to self-revising. Delegation is the fallback, not the default.
- The prose is a single short snippet — `lint` plus a direct edit may suffice;
  don't run the full rubric for a one-line comment.
- Never pass API keys on the CLI. Setup happens interactively via
  `writetighter config`; do not block the tighten workflow waiting for it.