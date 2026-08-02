# CLI Reference

The `writetighter` binary has five subcommands: `lint` (deterministic),
`revise` (contextual, needs a model), `prompt` (reusable revision guidance,
no model), `config` (interactive setup), and `profile` (bundle management).
This page catalogs each one.

## Overview

- **`lint`** never calls a model. It checks sentence length and dense
  paragraphs against your chosen `--kind`. Fast, auditable, always available.
- **`revise`** calls the model configured in `~/.config/writetighter/config.toml`.
  It returns rewrites and clarification questions — never edits your file.
- **`prompt`** prints the same revision instructions `revise` uses, without
  reading input or calling a model. Hand its output to a subagent that owns
  its own model call.
- **`config`** runs guided endpoint discovery and preflight once, interactively.
- **`profile`** lists, verifies, or installs profile bundles. The default,
  `software-docs-en@0.4.0`, ships embedded in the binary — you never need to
  pass `--profile` unless you're testing a different one.

`--text`, `--stdin`, and file/directory paths are mutually exclusive across
every command that reads input. Prefer `--stdin` for large or sensitive
content — `--text` can land in shell history.

## writetighter lint

Runs deterministic profile rules. Exits `0` unless `--fail-on` says otherwise.

```
writetighter lint (PATH... | --stdin | --text STRING) [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kind` | `description` | Document kind — sets the sentence-length limit (see table below) |
| `--format` | `human` | `human`, `json`, or `agent` |
| `--fail-on` | `none` | `none` (always exit 0), `warning`, or `error` |
| `--profile` | embedded default | Pin a specific `ID@VERSION`; rarely needed |

### Kinds

| `--kind` | Use for | Sentence limit |
|---|---|---:|
| `pr` | PR titles/bodies, commit bodies | 20 words |
| `procedure` | Step-by-step instructions | 20 words |
| `description` | Prose docs, READMEs, plans | 25 words |
| `code-comment` | Inline comments, doc comments, TODOs | 25 words |
| `reference` | API, configuration, and lookup docs | 25 words |
| `decision` | ADRs, design decisions, tradeoff records | 25 words |
| `incident` | Incident reports, timelines, remediations | 25 words |
| `agent-instruction` | Skills, system prompts, delegation prompts | 25 words |
| `status-update` | Progress, delay, blocker, diagnostic updates | 25 words |

### Example

```sh
writetighter lint README.md --kind description --format json
```

## writetighter revise

Runs contextual revision with the model from your user config. Reads
independently of any `lint` result — concise text can still hedge, leave a
pronoun pointing at nothing, or bury the actual change.

```
writetighter revise (PATH... | --stdin | --text STRING) --kind KIND
```

Output is JSON only. Each item in `revisions[]` is a `rewrite`
(`source_text`, `replacement`, `principle_ids`, `reason`, `confidence`) or a
`clarification` (`source_text`, `question`) — never both. Top-level
`discarded_rewrites` counts model rewrites dropped for losing protected
content (commands, paths, numbers, defined terms).

### Example

```sh
printf '%s' "$DRAFT" | writetighter revise --stdin --kind procedure
```

Requires `writetighter config` to have run once. Without it, an interactive
`revise` starts setup automatically; a non-interactive one (like `--stdin` in
a script) fails with a `writetighter config` hint instead of hanging.

## writetighter prompt

Prints the same core and kind-specific revision instructions `revise` uses —
no input read, no model called, no config required. Use it when a wrapping
agent should delegate the actual model call to a subagent instead of running
`revise` itself.

```sh
writetighter prompt --kind code-comment
writetighter prompt --kind decision --format json
```

See [Agent Integration](/agent-integration) for the full delegation pattern.

## writetighter config

Interactive-only. Queries an OpenAI-compatible endpoint's model list, sends a
preflight request to confirm structured-output support, and writes
`~/.config/writetighter/config.toml` with `0600` permissions. Never accepts
an API key as a command-line argument — keys go through a masked prompt or an
`api_key_env` reference.

```sh
writetighter config
```

## writetighter profile

```sh
writetighter profile list
writetighter profile verify software-docs-en@0.4.0
writetighter profile install ./path/to/bundle
```

## Exit codes

| Code | Meaning |
|---|---|
| `0` | `lint` below `--fail-on` threshold, or `revise`/`prompt` succeeded |
| `1` | `lint` findings reached `--fail-on` |
| `2` | Usage, config, profile, or input error |
| `3` | A required `revise` model call or response failed |
