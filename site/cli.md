# CLI Reference

The `writetighter` binary provides five subcommands: `lint` (deterministic), `revise` (contextual), `prompt` (instructional), `config` (setup), and `profile` (management).

## Commands

- **`lint`** checks sentence length and paragraph density. No model required.
- **`revise`** uses your configured model to provide rewrites and clarifications.
- **`prompt`** prints revision instructions for delegation. No model required.
- **`config`** interactive setup for model endpoints.
- **`profile`** manages embedded or custom rule bundles.

`--text`, `--stdin`, and file/dir paths are mutually exclusive. Prefer `--stdin` for shell safety.

## writetighter lint

Checks deterministic profile rules. For `--kind code-comment`, explicit supported source files (`.go`, TypeScript/JavaScript variants, `.rs`, `.py`, and `.pyi`) are lexed so only cataloged comments are linted; findings map back to the original source ranges. `--stdin`, `--text`, and unsupported extensions retain prose-style linting.

```
writetighter lint (PATH... | --stdin | --text STRING) [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--kind` | `description` | Sets sentence-length limit (see table below) |
| `--format` | `human` | `human`, `json`, or `agent` |
| `--fail-on` | `none` | `none`, `warning`, or `error` |
| `--profile` | embedded default | Pin a specific `ID@VERSION` |

### Kinds

| `--kind` | Use for | Sentence limit |
|---|---|---:|
| `pr` | PR/commit bodies | 20 words |
| `procedure` | Instructions | 20 words |
| `description` | Prose/READMEs | 25 words |
| `code-comment` | Inline comments | 25 words |
| `reference` | API/Config docs | 25 words |
| `decision` | ADRs/Design docs | 25 words |
| `incident` | Incident reports | 25 words |
| `agent-instruction` | Skills/Prompts | 25 words |
| `status-update` | Status/Diagnostics | 25 words |

## writetighter revise

Provides contextual rewrites and clarifications. For `--kind code-comment`, explicit supported source files (`.go`, TypeScript/JavaScript variants, `.rs`, `.py`, and `.pyi`) use a whole-file, lexer-owned comment catalog: only cataloged comments can be findings, and reported source text/ranges are catalog-owned. `--stdin`, `--text`, and unsupported extensions keep the legacy prose-style code-comment path. Code-aware file review rejects `--reference` until bounded cross-file context is supported. Review remains advisory and never applies edits.

```
writetighter revise (PATH... | --stdin | --text STRING) --kind KIND
```

Output is JSON. Each item in `revisions[]` is either a `rewrite` or a `clarification`. 

## writetighter prompt

Prints revision instructions for subagent delegation.

```sh
writetighter prompt --kind code-comment
```

## writetighter config

Interactive setup for OpenAI-compatible endpoints. Writes to `~/.config/writetighter/config.toml`.

## writetighter profile

```sh
writetighter profile list
writetighter profile verify software-docs-en@0.4.0
writetighter profile install ./path/to/bundle
```

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | `lint` findings reached `--fail-on` |
| `2` | Usage, config, or input error |
| `3` | `revise` model call failed |
