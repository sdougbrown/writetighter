# Delegating revision on Claude Code

Use this path when `writetighter revise` is unconfigured (exits with code 3) or
no model endpoint is available. Claude Code can spawn a subagent via the
**Agent tool** to perform the contextual revision and return the structured
`revisions[]` JSON the core loop expects.

## The contract

The subagent must return JSON matching the `revise` output shape: an array of
`revisions[]`, each with `kind` (`"rewrite"` or `"clarification"`), `source_text`,
`replacement` (for rewrites) or `question` (for clarifications), and optionally
`principle_ids`, `reason`, `confidence` (0–1). Anything else is a protocol error
— ask the subagent to re-emit in that shape rather than parsing free text.

## Steps

1. Get the rubric the configured `revise` would have used:

   ```sh
   writetighter prompt --kind "$KIND" --format json
   ```

   This prints the kind-specific revision directions without reading input or
   requiring a model. Capture it.

2. Spawn a subagent with the **Agent tool**. Use a cheaper model for the
   mechanical rewrite pass (e.g. `model: haiku`) unless the prose is technical
   enough to warrant more:

   ```
   subagent_type: general-purpose
   model: haiku
   prompt: <built below>
   ```

   Build the prompt from three parts: the rubric from step 1, the full `$DRAFT`,
   and an instruction to return **only** the `revisions[]` JSON — no prose
   preamble, no markdown fences. Tell it the same hard rules `revise` enforces:
   a `replacement` must preserve every command, path, identifier, and number in
   `source_text`; a `clarification` is a question, never a guessed rewrite; drop
   any rewrite that loses protected content.

3. Parse the subagent's output as `revisions[]`. If it is not valid JSON in the
   expected shape, ask the subagent once to re-emit cleanly before falling back
   to applying only the `lint` results.

4. Hand the parsed `revisions[]` back to the core loop at **step 3** (apply what
   you can verify) — do not bypass the verification rules because the source is a
   subagent instead of `revise`.

## When not to delegate

- The user has a configured endpoint — use `revise` directly. Delegation is the
  fallback, not the default.
- The prose is a single short snippet — `lint` plus your own judgment may
  suffice; don't spawn a subagent for a one-line comment.
- `revise` failed with code 3 but the user wants setup — tell them to run
  `writetighter config` interactively; do not block the tighten workflow waiting
  for it, and never pass API keys on the CLI.