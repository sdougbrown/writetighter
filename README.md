# WriteTighter

WriteTighter is a local, standalone Go CLI that checks technical prose against versioned rule profiles. It returns machine-readable findings and can optionally request advisory rewrite suggestions from a local LLM.

**Status:** Private development. The embedded default is `software-docs-en@0.2.0`. This repository is not currently cleared for public redistribution.

## What WriteTighter does (and does not) do

WriteTighter is a **neutral checking host**. It does not have its own opinions; instead, it selects a versioned **profile** that supplies the applicable dictionary and rule policy.

### It checks

- **Sentence length:** Applies profile thresholds for PRs, procedures, and descriptions.
- **Dense paragraphs:** Reports candidate findings for blocks that exceed configured sentence or word counts.
- **Term policy:** Supports discouraged terms, canonical case, unknown terms, and project term bases when the selected profile enables those rules.

### It does not do
- **Grammar checking:** It is not a general-purpose grammar checker.
- **Automatic rewriting:** It identifies issues and suggests improvements via LLM, but it will never modify your files.
- **Automatic compliance:** It provides findings based on rules; it does not "certify" your documentation.
- **Broad prose analysis:** It focuses on specific, rule-based metrics rather than subjective "quality."

## Local Setup

To use WriteTighter from this checkout, build the binary locally:

```sh
go build -o writetighter ./cmd/writetighter
```

Run `go install ./cmd/writetighter` instead if your Go bin directory is already on
`PATH`.

### Current default policy

The embedded `software-docs-en@0.2.0` profile uses these limits:

| Document kind | Sentence limit |
|---|---:|
| `pr` | 20 words |
| `procedure` | 20 words |
| `description` | 25 words |

It also emits an informational candidate when a paragraph has more than three
sentences or more than 80 words. The profile carries a small reviewed dictionary,
but version 0.2.0 does not enable deterministic term findings yet.

## Usage

### Basic Checks

You can check text via `stdin` or by providing paths to files and directories.

**Check via stdin:**

```sh
echo "This change replaces the global mutex with per-repository locks so unrelated repository updates no longer block one another during mirror fetch operations." |
  ./writetighter check --stdin --kind pr
```

The human report includes the static finding:

```text
status: checked
warning CORE.SENTENCE_LENGTH Split this sentence into independently verifiable claims.
```

**Check one file:**

```sh
./writetighter check README.md --kind description --format human
```

**Check a directory of Markdown files:**
```sh
./writetighter check docs/ --kind description --format json
```

### Output Formats and Exit Codes

`--format` defaults to `human`. WriteTighter supports three output formats:
- `human`: Readable text for developers.
- `json`: Structured data for tool integration.
- `agent`: Compact, line-oriented output for coding agents.

**Exit Codes:**
- `0`: Check completed successfully; no findings reached the failure threshold.
- `1`: Check completed; findings reached the `--fail-on` threshold.
- `2`: Usage, configuration, profile, or input failure.
- `3`: The `--require-llm` flag was used, but the LLM stage failed or was skipped.

You can control failure behavior with `--fail-on`:
- `--fail-on none` (default): Always exit `0`.
- `--fail-on warning`: Exit `1` if any `warning` or `error` findings exist.
- `--fail-on error`: Exit `1` if any `error` findings exist.

### Profiles

WriteTighter uses profiles to define its logic. The embedded default is `software-docs-en@0.2.0`.

**Manage profiles:**
```sh
# List installed profiles (including the embedded one)
./writetighter profile list

# Verify a specific profile or bundle path
./writetighter profile verify software-docs-en@0.2.0

# Install a new profile bundle from a local path
./writetighter profile install ./path/to/bundle
```

Pin profiles as `ID@VERSION` to keep results consistent across environments.

### Configuration and Term-bases

WriteTighter searches upward from the current working directory for
`.writetighter.toml`. Pass `--config PATH` to bypass discovery.

**Project configuration example:**
```toml
[profile]
id = "software-docs-en"
version = "0.2.0"

[[terms]]
term = "hydrate"
parts_of_speech = ["verb"]
definition = "Restore serialized application state."

[[terms]]
term = "utilize"
parts_of_speech = ["verb"]
definition = "The name of the project's public API operation."
override = true
reason = "The published API uses this exact operation name."
```

An addition does not need `override`. A project term that conflicts with a
discouraged profile term must set `override = true` and provide a non-empty reason.
Project configuration cannot contain LLM settings or credentials.

## LLM Advisory (Optional)

You can opt-in to LLM-powered advice. This is **advisory only** and runs only for files or passages that have already triggered static findings.

**How it works:**
1. WriteTighter runs the static rules.
2. If a finding is found, it builds a compact rewrite rubric from reviewed profile
   dictionary entries. Corpus-only `observed` terms never become prompt policy.
3. It asks the model for the minimum context-aware rewrite, using literal
   alternatives only when they preserve technical meaning and treating
   guidance-only entries as grammatical recasts.
4. It discards suggestions that change or omit protected technical content such as
   code spans, identifiers, commands, paths, URLs, numbers, versions, product names,
   or defined project terms.
5. Surviving rewrites are advisory findings only; WriteTighter never applies them.

**Network Access:** For security, network access is disabled by default. You must explicitly include the `--llm` flag on **every** invocation to use an LLM.

**Example with a local OpenAI-compatible endpoint:**
```sh
./writetighter check docs/ \
  --llm \
  --llm-base-url http://sparky:4000/v1 \
  --llm-model gemma4 \
  --llm-response-mode json_object
```

### Security: Secret Handling
Never pass API keys as command-line arguments. WriteTighter reads the key from an environment variable defined in your configuration via `api_key_env`.

Put machine-specific LLM settings in
`~/.config/writetighter/config.toml` (or the matching XDG config path):

```toml
[llm]
provider = "openai-compatible"
base_url = "http://sparky:4000/v1"
model = "gemma4"
api_key_env = "SPARKY_API_KEY"
```

## Practical Workflows

**Checking PR descriptions:**
```sh
./writetighter check pr_description.md --kind pr
```

**Checking recent Markdown plans:**
```sh
./writetighter check plans/*.md --kind description --format json
```

## Known Limitations (Pre-release)

As this is a private development build, please be aware of the following:
- **Profile Rights:** Public redistribution of profiles is currently restricted.
- **Heuristic Splitting:** Sentence and paragraph splitting uses heuristics and may occasionally miscalculate spans.
- **Narrow Policy:** The current profile has only two reviewed lexical-policy entries and does not yet enable its deterministic term rules.
- **Precision Gate:** The automated precision/recall for rule enforcement is still being calibrated.
- **No Auto-fix:** The tool identifies issues but does not automatically rewrite files.

## Documentation

- [Profile model and term-base boundaries](docs/decisions/0002-profile-model.md)
- [Product naming decision](docs/decisions/0001-product-name.md)
- [Data provenance and rights](docs/data-provenance.md)
- [Agent workflow](docs/agent-workflow.md)
