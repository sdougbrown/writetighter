# Agent Workflow

This document describes how coding agents interact with WriteTighter.

## Parent-run post-writer workflow

When a parent or dispatcher sends docs-writer to produce documentation, the
parent MUST run WriteTighter over the changed prose before accepting the result:

1. Dispatch docs-writer normally (via `@docs-writer` or `Agent(model: sonnet, ...)`).
2. After the writer finishes, run:
   ```sh
   writetighter check PATH ... --kind description --format json
   ```
3. Send the compact report back to docs-writer for one bounded revision pass.
4. Rerun the static check before acceptance.

Docs-writer never has shell access. The parent owns the check/review loop.

## Agent usage examples

### Check PR body

```sh
writetighter check --stdin --kind pr --format json \
  --profile software-docs-en@0.2.0 <<'EOF'
<PR title and body>
EOF
```

Agents fix only findings they can verify and rerun the static check. A second
`--llm` pass is permitted only when policy allows the configured endpoint to
receive the text.

### Check documentation

```sh
writetighter check docs/ --kind description --format json --fail-on none
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
2. Runs `writetighter check` over the produced files.
3. Iterates with docs-writer on findings.
4. Re-checks and accepts when findings are resolved or documented as
   acceptable exceptions.

See also:

- [Post-dispatch checklist](../references/post-dispatch-checklist.md) in
  the docs-writer skill
- [Configuration guide](configuration.md)
