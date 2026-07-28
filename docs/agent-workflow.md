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
4. Rerun the static check before acceptance.

Docs-writer never has shell access. The parent owns the check/review loop.

## Agent usage examples

### Check PR body

```sh
writetighter lint --stdin --kind pr --format json \
  --profile software-docs-en@0.2.0 <<'EOF'
<PR title and body>
EOF
```

Agents fix only findings they can verify and rerun `lint`. When the configured
endpoint is permitted to receive the text, a primary agent may request structured
revision advice without granting file-write access:

```sh
writetighter revise PATH --kind description --format json
```

The response contains source ranges plus rewrite suggestions or clarification
questions. The primary agent decides whether and how to edit; `revise` never
changes the target file. Exit code 3 means the required model call or response
failed, so an empty revision list must not be inferred from a failed process.

### Check documentation

```sh
writetighter lint docs/ --kind description --format json --fail-on none
```

### Verify embedded profile

```sh
writetighter profile verify software-docs-en@0.2.0 --format json
```

### List available profiles

```sh
writetighter profile list
```

## Precision gating

Checks are advisory by default. Enable required static checks only
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
- [README configuration and command guide](../README.md#llm-advisory-two-workflows)
