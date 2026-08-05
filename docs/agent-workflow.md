# Agent Workflow

This document describes how coding agents interact with WriteTighter.

## Parent-run post-writer workflow

When a parent or dispatcher sends docs-writer to produce documentation, the
parent MUST run WriteTighter over the changed prose before accepting the result:

1. Dispatch docs-writer normally (via `@docs-writer` or `Agent(model: sonnet, ...)`).
2. After the writer finishes, run:
   ```sh
   writetighter lint PATH ... --kind description --format json
   ```
3. Send the compact report back to docs-writer for one bounded revision pass.
4. Rerun `lint` before acceptance.

Docs-writer never has shell access. The parent owns the lint/review loop.

## Agent usage examples

### Lint PR body

```sh
writetighter lint --stdin --kind pr --format json \
  --profile software-docs-en@0.4.0 <<'EOF'
<PR title and body>
EOF
```

Agents fix only findings they can verify and rerun `lint`. When the configured
endpoint is permitted to receive the text, a primary agent may request structured
revision advice without granting file-write access. A human can create and preflight
machine-local model settings with `writetighter config`. Agents must not drive that
interactive workflow; without configuration, `revise` exits with a setup hint.

```sh
writetighter revise PATH --kind description --format json
```

The response contains source ranges plus rewrite suggestions or clarification
questions. The primary agent decides whether and how to edit; `revise` never
changes the target file. Exit code 3 means the required model call or response
failed, so an empty revision list must not be inferred from a failed process.

### Lint documentation

```sh
writetighter lint docs/ --kind description --format json --fail-on none
```

### Verify embedded profile

```sh
writetighter profile verify software-docs-en@0.4.0 --format json
```

### List available profiles

```sh
writetighter profile list
```

## References (optional)

When the document being revised needs broader context, you can provide
reference files with `--reference PATH`. Reference material is read-only
context; it is never linted or returned as a rewrite target. The contextual
revision can use reference facts to produce grounded rewrites instead of
asking for clarification.

```sh
writetighter revise docs/ --reference style-guide.md --format json
```

Reference paths may be files or directories. When combined with `revise`, the
`reference_context` field in the response reports metadata about the reference
material without exposing its content.

## Precision gating

Lint findings are advisory by default. Enable required rules only
rule-by-rule after the documented precision threshold passes (at least 100
labeled opportunities and 90% precision per rule).

## Interaction with docs-writer

The docs-writer skill does not have shell access. It cannot invoke
WriteTighter directly. The parent or dispatcher:

1. Runs docs-writer to produce documentation.
2. Runs `writetighter lint` over the produced files.
3. Iterates with docs-writer on findings.
4. Re-checks and accepts when findings are resolved or documented as
   acceptable exceptions.

See also:

- [Post-dispatch checklist](../references/post-dispatch-checklist.md) in
  the docs-writer skill
- [README configuration and command guide](../README.md#local-revision-model)
